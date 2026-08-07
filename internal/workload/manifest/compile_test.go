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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompile_StripsWorkloadID(t *testing.T) {
	m, err := Parse([]byte(`workloadId: 68b0aaaa0000000000000001
name: my-app
artifactId: 68b0bbbb0000000000000002
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	assert.Equal(t, "68b0aaaa0000000000000001", compiled.WorkloadID)
	assert.JSONEq(t, `{"name": "my-app", "artifactId": "68b0bbbb0000000000000002"}`, string(compiled.Payload))
}

// The shorthand must compile to exactly the object the workload package's
// EnvironmentVar marshals to: the expected JSON is built from that struct, so
// a drifted field tag fails here instead of at the API.
func TestCompile_ExpandsCredentialShorthand(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
artifact:
  spec:
    containerGroups:
      - containers:
          - name: primary
            imageUri: a:1
            environmentVars:
              - name: OPENAI_API_KEY
                value: dr-credential:68f0cccc0000000000000003/apiToken
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	expandedVar, err := json.Marshal(workload.EnvironmentVar{
		Source:         workload.EnvironmentVarSourceDRCredential,
		Name:           "OPENAI_API_KEY",
		DRCredentialID: "68f0cccc0000000000000003",
		Key:            "apiToken",
	})
	require.NoError(t, err)

	assert.JSONEq(t, fmt.Sprintf(`{
		"name": "my-app",
		"artifact": {
			"spec": {
				"containerGroups": [{
					"containers": [{
						"name": "primary",
						"imageUri": "a:1",
						"environmentVars": [%s]
					}]
				}]
			}
		}
	}`, expandedVar), string(compiled.Payload))

	require.Len(t, compiled.CredentialRefs, 1)

	ref := compiled.CredentialRefs[0]
	assert.Equal(t, "68f0cccc0000000000000003", ref.CredentialID)
	assert.Equal(t, "apiToken", ref.Key)
	assert.Equal(t, "OPENAI_API_KEY", ref.EnvName)
	assert.Positive(t, ref.Line)
}

func TestCompile_ObjectFormPassthroughAndCollected(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
artifact:
  spec:
    containerGroups:
      - containers:
          - name: primary
            imageUri: a:1
            environmentVars:
              - name: OPENAI_API_KEY
                source: dr-credential
                drCredentialId: 68f0cccc0000000000000003
                key: apiToken
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	var payload map[string]any

	require.NoError(t, json.Unmarshal(compiled.Payload, &payload))
	assert.NotContains(t, string(compiled.Payload), credentialShorthandPrefix+"68f0")

	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, "68f0cccc0000000000000003", compiled.CredentialRefs[0].CredentialID)
	assert.Equal(t, "apiToken", compiled.CredentialRefs[0].Key)
}

// Unknown blocks pass through untouched so platform additions need no CLI
// release.
func TestCompile_UnknownKeysSurvive(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
artifactId: abc
futureTopLevel:
  anything:
    - 1
    - two
artifact: null
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"name": "my-app",
		"artifactId": "abc",
		"futureTopLevel": {"anything": [1, "two"]},
		"artifact": null
	}`, string(compiled.Payload))
}

func TestCompile_MalformedShorthandFails(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
artifact:
  spec:
    containerGroups:
      - containers:
          - imageUri: a:1
            environmentVars:
              - name: KEY
                value: dr-credential:no-key-part
`), "")
	require.NoError(t, err)

	_, err = m.Compile()
	require.Error(t, err)

	assert.Contains(t, err.Error(), "must be dr-credential:<credential-id>/<key>")
}

// The compiled payload minus the CLI keys must be valid workload-create
// input; holding that property here is the down-payment on the end-to-end
// smoke test that deploys a wizard-written manifest.
func TestCompile_PayloadIsValidCreateSpec(t *testing.T) {
	m, err := Parse([]byte(validManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	assert.NoError(t, workload.ValidateWorkloadCreateRequest(compiled.Payload))
}
