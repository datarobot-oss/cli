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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func portableFixture() Live {
	return Live{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Name:       "app",
		Spec: map[string]any{
			"containerGroups": []any{map[string]any{
				"containers": []any{map[string]any{
					"name":    "primary",
					"primary": true,
					"environmentVars": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "debug"},
						map[string]any{"name": "API_KEY", "source": "api-key"},
						map[string]any{
							"name": "TOKEN", "source": "dr-credential",
							"drCredentialId": "68f0cccc0000000000000003", "key": "apiToken",
						},
						map[string]any{
							"name": "OTHER", "source": "dr-credential",
							"drCredentialId": "68f0cccc0000000000000004", "key": "apiToken",
						},
					},
				}},
			}},
		},
		Runtime: map[string]any{},
	}
}

func TestLivePortable_UnbindsAndResetsCredentialReferences(t *testing.T) {
	original := portableFixture()

	portable, pending := original.Portable()

	assert.Empty(t, portable.WorkloadID)
	assert.Equal(t, 2, pending)

	vars := primaryContainerOf(portable.Spec)[keyEnvironmentVars].([]any)

	assert.Equal(t, CredentialPlaceholder, vars[2].(map[string]any)[keyDRCredentialID])
	assert.Equal(t, CredentialPlaceholder, vars[3].(map[string]any)[keyDRCredentialID])
	// The key stays: it names the field inside the credential, not an id.
	assert.Equal(t, "apiToken", vars[2].(map[string]any)[keyKey])
	// Everything that is not a stored credential is carried as it was.
	assert.Equal(t, map[string]any{"name": "LOG_LEVEL", "value": "debug"}, vars[0])
	assert.Equal(t, map[string]any{"name": "API_KEY", "source": "api-key"}, vars[1])
}

func TestLivePortable_LeavesTheOriginalAlone(t *testing.T) {
	original := portableFixture()

	_, _ = original.Portable()

	vars := primaryContainerOf(original.Spec)[keyEnvironmentVars].([]any)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", original.WorkloadID)
	assert.Equal(t, "68f0cccc0000000000000003", vars[2].(map[string]any)[keyDRCredentialID])
}

func TestLivePortable_RendersAManifestWithNoBinding(t *testing.T) {
	portable, _ := portableFixture().Portable()

	content, err := portable.Render()
	require.NoError(t, err)

	parsed, err := Parse(content, "")
	require.NoError(t, err)

	assert.Empty(t, parsed.WorkloadID())
	assert.NotContains(t, string(content), "68f0cccc0000000000000003")
	assert.Contains(t, string(content), CredentialPlaceholder)
}
