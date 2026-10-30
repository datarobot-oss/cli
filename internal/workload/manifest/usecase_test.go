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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const useCaseFixture = `name: my-app
importance: low # low | moderate | high | critical

artifactId: 68a1

runtime:
  enclaveSelectionPolicy: manual
  enclaves: [prod-east]
`

// The key reaches the create payload unchanged: it is the endpoint's own.
func TestSetUseCase_LandsAfterImportanceAndInThePayload(t *testing.T) {
	out, err := SetUseCase([]byte(useCaseFixture), "68b0aa11bb22cc33dd44ee55", false)
	require.NoError(t, err)

	assert.Contains(t, string(out), "importance: low # low | moderate | high | critical\n"+
		"useCaseId: 68b0aa11bb22cc33dd44ee55\nartifactId: 68a1\n")

	m, err := Parse(out, "")
	require.NoError(t, err)
	assert.Equal(t, "68b0aa11bb22cc33dd44ee55", m.UseCaseID())

	compiled, err := m.Compile()
	require.NoError(t, err)

	var payload map[string]any

	require.NoError(t, json.Unmarshal(compiled.Payload, &payload))
	assert.Equal(t, "68b0aa11bb22cc33dd44ee55", payload["useCaseId"])
}

// A policy the file already names is the user's choice, pin included.
func TestSetUseCase_KeepsAPolicyAlreadyThere(t *testing.T) {
	out, err := SetUseCase([]byte(useCaseFixture), "68b0aa11bb22cc33dd44ee55", true)
	require.NoError(t, err)

	assert.Contains(t, string(out), "enclaveSelectionPolicy: manual")
	assert.NotContains(t, string(out), "availability")
}

func TestSetUseCase_AddsAvailabilityAndReplacesTheID(t *testing.T) {
	out, err := SetUseCase([]byte("name: my-app\nuseCaseId: old\n"), "68b0aa11bb22cc33dd44ee55", true)
	require.NoError(t, err)

	assert.Equal(t, "name: my-app\nuseCaseId: 68b0aa11bb22cc33dd44ee55\n\nruntime:\n"+
		"  enclaveSelectionPolicy: availability\n", string(out))
}
