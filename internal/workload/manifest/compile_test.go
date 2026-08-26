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
	assert.NotContains(t, string(compiled.Payload), CredentialShorthandPrefix+"68f0")

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

// A manifest may define a shared environmentVars block anywhere and alias it
// into containerGroups, e.g. to keep several containers' variables in sync.
// The anchor itself sits under a key other than environmentVars, so the only
// place the credential walk can find this reference at all is by resolving
// the alias at its environmentVars usage.
func TestCompile_ExpandsCredentialShorthandThroughAlias(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
definitions:
  sharedEnv: &sharedEnv
    - name: OPENAI_API_KEY
      value: dr-credential:68f0cccc0000000000000003/apiToken
artifact:
  spec:
    containerGroups:
      - containers:
          - name: primary
            imageUri: a:1
            environmentVars: *sharedEnv
`), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	require.Len(t, compiled.CredentialRefs, 1)
	assert.Equal(t, "68f0cccc0000000000000003", compiled.CredentialRefs[0].CredentialID)

	var payload struct {
		Artifact struct {
			Spec struct {
				ContainerGroups []struct {
					Containers []struct {
						EnvironmentVars []workload.EnvironmentVar `json:"environmentVars"`
					} `json:"containers"`
				} `json:"containerGroups"`
			} `json:"spec"`
		} `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(compiled.Payload, &payload))

	envVars := payload.Artifact.Spec.ContainerGroups[0].Containers[0].EnvironmentVars
	require.Len(t, envVars, 1)
	assert.Equal(t, workload.EnvironmentVar{
		Source:         workload.EnvironmentVarSourceDRCredential,
		Name:           "OPENAI_API_KEY",
		DRCredentialID: "68f0cccc0000000000000003",
		Key:            "apiToken",
	}, envVars[0])
}

func TestCompile_MalformedShorthandThroughAliasFails(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
definitions:
  sharedEnv: &sharedEnv
    - name: KEY
      value: dr-credential:no-key-part
artifact:
  spec:
    containerGroups:
      - containers:
          - imageUri: a:1
            environmentVars: *sharedEnv
`), "")
	require.NoError(t, err)

	_, err = m.Compile()
	require.Error(t, err)

	assert.Contains(t, err.Error(), "must be dr-credential:<credential-id>/<key>")
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

// buildManifest asks the platform to build, which is the only case where the
// artifact has to be created on its own before a workload can reference it.
const buildManifest = `name: my-app
importance: low
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageBuildConfig:
              dockerfile:
                source: provided
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
`

// The manifest carries the artifact-create request verbatim, so lifting the
// block out is the whole of the translation.
func TestArtifactPayload_IsTheBlockItself(t *testing.T) {
	m, err := Parse([]byte(buildManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	payload, err := compiled.ArtifactPayload()
	require.NoError(t, err)

	var artifact map[string]any

	require.NoError(t, json.Unmarshal(payload, &artifact))
	assert.Equal(t, "my-app-artifact", artifact["name"])
	assert.Contains(t, artifact, "spec")
	assert.NotContains(t, artifact, "runtime", "the sizing half is the workload's, not the artifact's")
}

// Credential shorthand is expanded on the way out here as it is everywhere
// else, because this payload reaches the API without passing through the
// workload create.
func TestArtifactPayload_CarriesExpandedCredentials(t *testing.T) {
	m, err := Parse([]byte(`name: my-app
artifact:
  name: my-app-artifact
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

	payload, err := compiled.ArtifactPayload()
	require.NoError(t, err)

	assert.Contains(t, string(payload), "drCredentialId")
	assert.NotContains(t, string(payload), CredentialShorthandPrefix)
}

func TestArtifactPayload_NoBlockToCreate(t *testing.T) {
	m, err := Parse([]byte("name: my-app\nartifactId: 68b0bbbb0000000000000002\n"), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	_, err = compiled.ArtifactPayload()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no artifact block")
}

// The spec on its own is what an update takes: the artifact already has a name
// and a type, and the platform owns the type once it exists.
func TestArtifactSpecPayload_IsTheSpecAlone(t *testing.T) {
	m, err := Parse([]byte(buildManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	payload, err := compiled.ArtifactSpecPayload()
	require.NoError(t, err)

	var spec map[string]any

	require.NoError(t, json.Unmarshal(payload, &spec))
	assert.Contains(t, spec, "containerGroups")
	assert.NotContains(t, spec, "name", "the name is the artifact's, not its spec's")
	assert.Equal(t, "my-app-artifact", compiled.ArtifactName, "which is lifted out for the copy to be named")
}

// A manifest bound by id describes no block, so there is nothing to lift.
func TestCompile_ArtifactBoundByIDHasNoBlockToRead(t *testing.T) {
	m, err := Parse([]byte("name: my-app\nartifactId: 68b0bbbb0000000000000002\n"), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	assert.Empty(t, compiled.ArtifactName)

	_, err = compiled.ArtifactSpecPayload()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no artifact block")
}

// The mirror of ArtifactPayload: the sizing half on its own, with nothing in
// it about what the workload runs.
func TestRuntimePayload_IsTheBlockItself(t *testing.T) {
	m, err := Parse([]byte(buildManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	payload, err := compiled.RuntimePayload()
	require.NoError(t, err)

	var runtime map[string]any

	require.NoError(t, json.Unmarshal(payload, &runtime))
	assert.Contains(t, runtime, "containerGroups")
	assert.NotContains(t, runtime, "spec", "what it runs is the artifact's half")
	assert.NotContains(t, runtime, "name", "the workload's own name is not a runtime setting")
}

func TestRuntimePayload_NoBlockToApply(t *testing.T) {
	m, err := Parse([]byte("name: my-app\nartifactId: 68b0bbbb0000000000000002\n"), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	_, err = compiled.RuntimePayload()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no runtime block")
}

// The two forms are mutually exclusive in the API as they are in the file, so
// binding removes the block rather than leaving it beside the id. Sending
// both would ask the platform for a second, empty artifact.
func TestBindArtifact_ReplacesTheInlineBlock(t *testing.T) {
	m, err := Parse([]byte(buildManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	payload, err := compiled.BindArtifact("68a0000000000000000000a1")
	require.NoError(t, err)

	var bound map[string]any

	require.NoError(t, json.Unmarshal(payload, &bound))
	assert.Equal(t, "68a0000000000000000000a1", bound["artifactId"])
	assert.NotContains(t, bound, "artifact")
	assert.Equal(t, "my-app", bound["name"])
	assert.Contains(t, bound, "runtime")
	assert.Contains(t, bound, "importance", "everything the file said about the workload still travels")
}

// Each call edits its own copy, so one deploy's binding cannot leak into
// another's payload.
func TestBindArtifact_LeavesTheCompiledPayloadAlone(t *testing.T) {
	m, err := Parse([]byte(buildManifest), "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	_, err = compiled.BindArtifact("68a0000000000000000000a1")
	require.NoError(t, err)

	assert.Contains(t, string(compiled.Payload), `"artifact"`)
	assert.NotContains(t, string(compiled.Payload), "artifactId")
}
