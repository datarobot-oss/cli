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

package workload

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyEnclavePin_AddsRuntimeWhenAbsent(t *testing.T) {
	pinned, err := ApplyEnclavePin([]byte(`{"name":"my-app","artifactId":"art-1"}`), "prod-east")
	require.NoError(t, err)

	var doc map[string]any

	require.NoError(t, json.Unmarshal(pinned, &doc))
	assert.Equal(t, "my-app", doc["name"])

	runtime, ok := doc["runtime"].(map[string]any)

	require.True(t, ok)
	assert.Equal(t, EnclaveSelectionPolicyManual, runtime["enclaveSelectionPolicy"])
	assert.Equal(t, []any{"prod-east"}, runtime["enclaves"])
}

func TestApplyEnclavePin_PreservesExistingRuntimeFields(t *testing.T) {
	spec := `{
		"name": "my-app",
		"artifactId": "art-1",
		"runtime": {
			"containerGroups": [{
				"name": "default",
				"replicaCount": 2,
				"containers": [{"name": "app", "resourceAllocation": {"cpu": 1, "memory": 4294967296}}]
			}]
		}
	}`

	pinned, err := ApplyEnclavePin([]byte(spec), "prod-east")
	require.NoError(t, err)

	var doc map[string]any

	require.NoError(t, json.Unmarshal(pinned, &doc))

	runtime := doc["runtime"].(map[string]any)

	assert.Equal(t, EnclaveSelectionPolicyManual, runtime["enclaveSelectionPolicy"])
	assert.NotNil(t, runtime["containerGroups"], "existing runtime content must survive the pin")

	// The large memory integer must survive re-encoding verbatim rather than
	// decaying into scientific notation via float64.
	assert.Contains(t, string(pinned), "4294967296")
}

func TestApplyEnclavePin_TrimsTheName(t *testing.T) {
	pinned, err := ApplyEnclavePin([]byte(`{"name":"a"}`), "  prod-east  ")
	require.NoError(t, err)
	assert.Contains(t, string(pinned), `"enclaves":["prod-east"]`)
}

func TestApplyEnclavePin_RejectsBlankName(t *testing.T) {
	for _, name := range []string{"", "   "} {
		_, err := ApplyEnclavePin([]byte(`{"name":"a"}`), name)
		require.Error(t, err, "name %q", name)
		assert.Contains(t, err.Error(), "non-blank")
	}
}

func TestApplyEnclavePin_RejectsSpecWithSelectionPolicy(t *testing.T) {
	spec := `{"name":"a","runtime":{"enclaveSelectionPolicy":"availability"}}`

	_, err := ApplyEnclavePin([]byte(spec), "prod-east")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime.enclaveSelectionPolicy")
}

func TestApplyEnclavePin_RejectsSpecWithEnclaves(t *testing.T) {
	// An empty list still counts: presence is the conflict, since the file
	// said something about placement on purpose.
	spec := `{"name":"a","runtime":{"enclaves":[]}}`

	_, err := ApplyEnclavePin([]byte(spec), "prod-east")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime.enclaves")
}

func TestApplyEnclavePin_RejectsNonObjectRuntime(t *testing.T) {
	_, err := ApplyEnclavePin([]byte(`{"name":"a","runtime":[]}`), "prod-east")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'runtime' must be an object")
}

func TestApplyEnclavePin_NullRuntimeIsAbsent(t *testing.T) {
	pinned, err := ApplyEnclavePin([]byte(`{"name":"a","runtime":null}`), "prod-east")
	require.NoError(t, err)
	assert.Contains(t, string(pinned), `"enclaves":["prod-east"]`)
}

func TestApplyEnclavePin_RejectsInvalidJSON(t *testing.T) {
	_, err := ApplyEnclavePin([]byte(`{`), "prod-east")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid spec")
}
