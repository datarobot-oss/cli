// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manifest

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A commented-out block after the last item of a nested sequence is re-emitted
// two columns further right by yaml.v3, so replacing an id must not re-encode.
const nestedCommentManifest = `workloadId: 6ad3c5ed190d72e5948d8051 # managed by the CLI
name: deepagent-v2-base
importance: low # low | moderate | high | critical
artifact:
  name: deepagent-v2-base-artifact
  type: agent
  spec:
    containerGroups:
      - name: default
        containers:
          - name: app
            env:
              - source: string
                name: OTEL_TRACES_ENABLED
                value: "true"
              # Mem0 long-term memory: off until uncommented.
              # - source: string
              #   name: MEM0_ENABLED
              #   value: "true"

runtime:
  replicaCount: 1
`

func TestWriteWorkloadID_ReplaceKeepsNestedCommentColumns(t *testing.T) {
	path := writeManifest(t, t.TempDir(), nestedCommentManifest)

	require.NoError(t, WriteWorkloadID(path, "6adb9de43bff6a3b37d1b9d7"))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	want := strings.Replace(nestedCommentManifest, "6ad3c5ed190d72e5948d8051", "6adb9de43bff6a3b37d1b9d7", 1)
	assert.Equal(t, want, string(got))
}

// Quoted values and CRLF files are spliced as written, never corrupted.
func TestWriteWorkloadID_ReplaceQuotedAndCRLF(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"double quoted": {
			in:   "workloadId: \"6ad3c5ed190d72e5948d8051\" # managed by the CLI\nname: x\n",
			want: "workloadId: \"6adb9de43bff6a3b37d1b9d7\" # managed by the CLI\nname: x\n",
		},
		"single quoted": {
			in:   "workloadId: '6ad3c5ed190d72e5948d8051'\nname: x\n",
			want: "workloadId: '6adb9de43bff6a3b37d1b9d7'\nname: x\n",
		},
		"crlf": {
			in:   "workloadId: 6ad3c5ed190d72e5948d8051 # managed by the CLI\r\nname: x\r\n",
			want: "workloadId: 6adb9de43bff6a3b37d1b9d7 # managed by the CLI\r\nname: x\r\n",
		},
		"not first key": {
			in:   "name: x\n# bound by up\nworkloadId: 6ad3c5ed190d72e5948d8051\n",
			want: "name: x\n# bound by up\nworkloadId: 6adb9de43bff6a3b37d1b9d7\n",
		},
		"multi-byte text before the token": {
			in:   "{name: ééé, workloadId: 6ad3c5ed190d72e5948d8051}\n",
			want: "{name: ééé, workloadId: 6adb9de43bff6a3b37d1b9d7}\n",
		},
		"same id writes nothing": {
			in:   "workloadId: 6adb9de43bff6a3b37d1b9d7 # managed by the CLI\nname: x\n",
			want: "workloadId: 6adb9de43bff6a3b37d1b9d7 # managed by the CLI\nname: x\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), tc.in)

			require.NoError(t, WriteWorkloadID(path, "6adb9de43bff6a3b37d1b9d7"))

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// A value the splice cannot model still lands in the tree edit, which
// rebinds it and keeps the comment.
func TestWriteWorkloadID_ReplaceFallsBackToTreeEdit(t *testing.T) {
	for name, in := range map[string]string{
		"spaces in quotes": "workloadId: \"a b\" # managed by the CLI\nname: x\n",
		"block scalar":     "workloadId: |\n  6ad3c5ed190d72e5948d8051\n# bound by up\nname: x\n",
		"alias":            "anchor: &id other\nworkloadId: *id # managed by the CLI\nname: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), in)

			require.NoError(t, WriteWorkloadID(path, "6adb9de43bff6a3b37d1b9d7"))

			m, err := Load(path)
			require.NoError(t, err)
			assert.Equal(t, "6adb9de43bff6a3b37d1b9d7", m.WorkloadID())

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(got), "name: x")
			assert.True(t, strings.Contains(string(got), "managed by the CLI") || strings.Contains(string(got), "bound by up"))
		})
	}
}
