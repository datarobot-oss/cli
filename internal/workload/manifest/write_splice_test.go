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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

const oldID, newID = "6ad3c5ed190d72e5948d8051", "6adb9de43bff6a3b37d1b9d7"

func TestWriteWorkloadID_ReplaceKeepsNestedCommentColumns(t *testing.T) {
	path := writeManifest(t, t.TempDir(), nestedCommentManifest)

	require.NoError(t, WriteWorkloadID(path, newID))

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	want := strings.Replace(nestedCommentManifest, oldID, newID, 1)
	assert.Equal(t, want, string(got))
}

// Every case keeps the nested comment block a re-encode would move, so each
// one fails unless the splice actually ran.
func TestWriteWorkloadID_ReplaceQuotedAndCRLF(t *testing.T) {
	firstLine := "workloadId: " + oldID + " # managed by the CLI\n"
	rest := strings.TrimPrefix(nestedCommentManifest, firstLine)

	for name, in := range map[string]string{
		"double quoted":   `workloadId: "` + oldID + `" # managed by the CLI` + "\n" + rest,
		"single quoted":   "workloadId: '" + oldID + "'\n" + rest,
		"crlf":            strings.ReplaceAll(nestedCommentManifest, "\n", "\r\n"),
		"not first key":   rest + "# bound by up\nworkloadId: " + oldID + "\n",
		"byte order mark": "\xef\xbb\xbf" + nestedCommentManifest,
	} {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), in)

			require.NoError(t, WriteWorkloadID(path, newID))

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, strings.Replace(in, oldID, newID, 1), string(got))
		})
	}

	t.Run("same id writes nothing", func(t *testing.T) {
		path := writeManifest(t, t.TempDir(), nestedCommentManifest)

		require.NoError(t, WriteWorkloadID(path, oldID))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, nestedCommentManifest, string(got))
	})
}

// Column counts runes, so multi-byte text before the token is stepped over
// whole.
func TestTokenOffset_StepsRunes(t *testing.T) {
	data := []byte("name: x\n{a: ééé, b: c}\n")

	offset, ok := tokenOffset(data, 2, 13)
	require.True(t, ok)
	assert.Equal(t, "c}\n", string(data[offset:]))
}

// A position whose bytes do not hold the parsed value is refused, never
// overwritten.
func TestSpliceScalar_RefusesBytesThatDoNotMatch(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "abc", Line: 1, Column: 3}

	_, ok := spliceScalar([]byte("x: abc\n"), node, "xyz")
	assert.False(t, ok)
}

// The splice declines what it cannot place or write back exactly, and the
// tree edit still rebinds the file without touching anything else.
func TestWriteWorkloadID_SpliceDeclines(t *testing.T) {
	for name, tc := range map[string]struct{ in, id string }{
		// yaml.v3 also ends a line on a lone CR or LS, so a count of LFs
		// lands on the next line, here a name holding the same text.
		"lone CR before the binding": {
			in: "importance: low\rworkloadId: " + oldID + "\nname:       " + oldID + "\n", id: newID,
		},
		"line separator before the binding": {
			in: "importance: low\xe2\x80\xa8workloadId: " + oldID + "\nname:       " + oldID + "\n", id: newID,
		},
		// Written plain, an all-digit id would read back as a number.
		"all-digit id": {in: "workloadId: " + oldID + "\nname: x\n", id: "123456789012345678901234"},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), tc.in)

			data, err := os.ReadFile(path)
			require.NoError(t, err)

			_, root, err := soleDocument(data, path, "test", func(format string, args ...any) error {
				return fmt.Errorf(format, args...)
			})
			require.NoError(t, err)

			_, ok := spliceWorkloadID(data, root, tc.id)
			assert.False(t, ok)

			require.NoError(t, WriteWorkloadID(path, tc.id))

			m, err := Load(path)
			require.NoError(t, err)
			assert.Equal(t, tc.id, m.WorkloadID())

			if strings.Contains(tc.in, "name:       ") {
				assert.Equal(t, oldID, m.Name())
			}
		})
	}
}

// A value the splice cannot model still lands in the tree edit, which
// rebinds it and keeps the comment.
func TestWriteWorkloadID_ReplaceFallsBackToTreeEdit(t *testing.T) {
	for name, in := range map[string]string{
		"spaces in quotes": "workloadId: \"a b\" # managed by the CLI\nname: x\n",
		"block scalar":     "workloadId: |\n  " + oldID + "\n# bound by up\nname: x\n",
		"alias":            "anchor: &id other\nworkloadId: *id # managed by the CLI\nname: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeManifest(t, t.TempDir(), in)

			require.NoError(t, WriteWorkloadID(path, newID))

			m, err := Load(path)
			require.NoError(t, err)
			assert.Equal(t, newID, m.WorkloadID())

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(got), "name: x")
			assert.True(t, strings.Contains(string(got), "managed by the CLI") || strings.Contains(string(got), "bound by up"))
		})
	}
}
