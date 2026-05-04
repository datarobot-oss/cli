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

const testArtifactJSON = `{
	"id": "art-abc-123",
	"name": "my-agent",
	"status": "DRAFT",
	"spec": {
		"containerGroups": [
			{
				"containers": [
					{
						"codeRef": {
							"datarobot": {
								"catalogId": "cat-xyz-789",
								"catalogVersionId": "fedcba0987654321fedcba09"
							}
						}
					}
				]
			}
		]
	},
	"createdAt": "2026-04-01T08:00:00Z",
	"updatedAt": "2026-04-10T14:30:00Z"
}`

const testArtifactNoCodeRefJSON = `{
	"id": "art-abc-123",
	"name": "my-agent",
	"status": "DRAFT",
	"spec": {
		"containerGroups": [
			{
				"containers": [
					{}
				]
			}
		]
	},
	"createdAt": "2026-04-01T08:00:00Z",
	"updatedAt": "2026-04-10T14:30:00Z"
}`

func TestExtractCodeRef_Found(t *testing.T) {
	var artifact Artifact

	err := json.Unmarshal([]byte(testArtifactJSON), &artifact)
	require.NoError(t, err)

	codeRef := ExtractCodeRef(artifact)
	require.NotNil(t, codeRef)
	assert.Equal(t, "cat-xyz-789", codeRef.CatalogID)
	assert.Equal(t, "fedcba0987654321fedcba09", codeRef.CatalogVersionID)
}

func TestExtractCodeRef_NoContainerGroups(t *testing.T) {
	artifact := Artifact{Spec: Spec{ContainerGroups: nil}}
	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_EmptyContainers(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilCodeRef(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{CodeRef: nil}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilDatarobot(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{CodeRef: &CodeRef{Datarobot: nil}}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NotInResponse(t *testing.T) {
	var artifact Artifact

	err := json.Unmarshal([]byte(testArtifactNoCodeRefJSON), &artifact)
	require.NoError(t, err)

	assert.Nil(t, ExtractCodeRef(artifact))
}

// TestExtractCodeRef_PrefersPrimaryWhenSidecarFirst is the regression test
// for the read/write asymmetry: PatchArtifactCodeRef writes the
// primary=true container, so reads must follow it too — otherwise a
// sidecar appearing at index 0 makes display/init show stale catalog
// info immediately after a sync.
func TestExtractCodeRef_PrefersPrimaryWhenSidecarFirst(t *testing.T) {
	primary := true
	notPrimary := false

	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							Primary: &notPrimary,
							CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
								CatalogID: "sidecar-cat", CatalogVersionID: "sidecar-ver",
							}},
						},
						{
							Primary: &primary,
							CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
								CatalogID: "primary-cat", CatalogVersionID: "primary-ver",
							}},
						},
					},
				},
			},
		},
	}

	codeRef := ExtractCodeRef(artifact)
	require.NotNil(t, codeRef)
	assert.Equal(t, "primary-cat", codeRef.CatalogID)
	assert.Equal(t, "primary-ver", codeRef.CatalogVersionID)
}

func TestExtractCodeRef_FindsPrimaryAcrossGroups(t *testing.T) {
	primary := true

	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{}}},
				{
					Containers: []Container{
						{
							Primary: &primary,
							CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
								CatalogID: "cat-2", CatalogVersionID: "ver-2",
							}},
						},
					},
				},
			},
		},
	}

	codeRef := ExtractCodeRef(artifact)
	require.NotNil(t, codeRef)
	assert.Equal(t, "cat-2", codeRef.CatalogID)
}

// TestExtractCodeRef_PrimaryFalseDoesNotMatch confirms the *bool primary
// flag is interpreted strictly: a container with `primary: false` set
// explicitly is treated as a sidecar, not a candidate.
func TestExtractCodeRef_PrimaryFalseDoesNotMatch(t *testing.T) {
	notPrimary := false

	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							Primary: &notPrimary,
							CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
								CatalogID: "first", CatalogVersionID: "v1",
							}},
						},
					},
				},
			},
		},
	}

	// No container marked primary → fallback to [0][0], which still
	// returns "first". Asserts the fallback path remains live.
	codeRef := ExtractCodeRef(artifact)
	require.NotNil(t, codeRef)
	assert.Equal(t, "first", codeRef.CatalogID)
}

func TestParseArtifactStatus(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"draft", "draft"},
		{"DRAFT", "draft"},
		{"Locked", "locked"},
		{"locked", "locked"},
	}

	for _, c := range cases {
		got, err := ParseArtifactStatus(c.in)
		require.NoError(t, err, "input %q", c.in)
		assert.Equal(t, c.want, got, "input %q", c.in)
	}
}

func TestParseArtifactStatus_Invalid(t *testing.T) {
	_, err := ParseArtifactStatus("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
	assert.Contains(t, err.Error(), "bogus")
}

// TestIsLocked guards the case-insensitive comparison so the locked-artifact
// guard cannot regress: the API wire format is uppercase ("LOCKED"), but the
// constant is lowercase, so a plain == check would silently let locked
// artifacts through.
func TestIsLocked(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"LOCKED", true},
		{"locked", true},
		{"Locked", true},
		{"DRAFT", false},
		{"draft", false},
		{"", false},
	}

	for _, c := range cases {
		a := &Artifact{Status: c.status}
		assert.Equal(t, c.want, a.IsLocked(), "status %q", c.status)
	}
}

const validMinimalSpec = `{
	"name": "my-agent",
	"spec": {
		"containerGroups": [{
			"containers": [{
				"imageUri": "nginx:latest",
				"port": 8080,
				"resourceRequest": {"cpu": 1, "memory": 536870912}
			}]
		}]
	}
}`

func TestValidateCreateRequest_MinimalValid(t *testing.T) {
	require.NoError(t, ValidateCreateRequest([]byte(validMinimalSpec)))
}

func TestValidateCreateRequest_FullValid(t *testing.T) {
	spec := `{
		"name": "my-agent",
		"description": "demo",
		"spec": {
			"containerGroups": [{
				"containers": [{
					"imageUri": "nginx:latest",
					"port": 8080,
					"resourceRequest": {"cpu": 1, "memory": 536870912},
					"codeRef": {"datarobot": {"catalogId": "c", "catalogVersionId": "v"}}
				}]
			}]
		}
	}`

	require.NoError(t, ValidateCreateRequest([]byte(spec)))
}

func TestValidateCreateRequest_Errors(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantSub string
	}{
		{
			"missing name",
			`{"spec":{"containerGroups":[{"containers":[{}]}]}}`,
			"'name' is missing",
		},
		{
			"empty name",
			`{"name":"","spec":{"containerGroups":[{"containers":[{}]}]}}`,
			"'name' is missing",
		},
		{
			"missing spec",
			`{"name":"x"}`,
			"'spec.containerGroups'",
		},
		{
			"empty container groups",
			`{"name":"x","spec":{"containerGroups":[]}}`,
			"'spec.containerGroups'",
		},
		{
			"empty containers in group",
			`{"name":"x","spec":{"containerGroups":[{"containers":[]}]}}`,
			"[0].containers",
		},
		{
			"unknown top-level field",
			`{"nme":"x","spec":{"containerGroups":[{"containers":[{}]}]}}`,
			`unknown field "nme"`,
		},
		{
			"unknown nested field",
			`{"name":"x","spec":{"containerGroups":[{"containers":[{"imageUrl":"x"}]}]}}`,
			`unknown field "imageUrl"`,
		},
		{
			"wrong type cpu",
			`{"name":"x","spec":{"containerGroups":[{"containers":[{"resourceRequest":{"cpu":"1","memory":1}}]}]}}`,
			"invalid spec",
		},
		{
			"not json",
			`not json`,
			"invalid spec",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateCreateRequest([]byte(c.spec))
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantSub)
		})
	}
}

func TestSetPrimaryCodeRefInRawArtifact(t *testing.T) {
	t.Run("OverwritesPrimaryCodeRef_LeavesSidecarsAlone", func(t *testing.T) {
		// Sidecar at containers[0]; primary at containers[1]. Only the
		// primary's codeRef should be repointed; the sidecar's codeRef
		// must survive untouched.
		raw := map[string]any{
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{
						"containers": []any{
							map[string]any{
								"primary": false,
								"codeRef": map[string]any{
									"datarobot": map[string]any{
										"catalogId":        "sidecar-cat",
										"catalogVersionId": "sidecar-ver",
									},
								},
								"imageUri": "sidecar-image",
							},
							map[string]any{
								"primary": true,
								"codeRef": map[string]any{
									"datarobot": map[string]any{
										"catalogId":        "old-cat",
										"catalogVersionId": "old-ver",
									},
								},
								"imageUri": "primary-image",
							},
						},
					},
				},
			},
		}

		require.NoError(t, setPrimaryCodeRefInRawArtifact(raw, "new-cat", "new-ver"))

		containers := raw["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
		sidecar := containers[0].(map[string]any)
		primary := containers[1].(map[string]any)

		// Primary repointed.
		primaryRef := primary["codeRef"].(map[string]any)
		assert.Equal(t, "datarobot", primaryRef["type"])
		assert.Equal(t, "datarobot", primaryRef["provider"])
		dr := primaryRef["datarobot"].(map[string]any)
		assert.Equal(t, "new-cat", dr["catalogId"])
		assert.Equal(t, "new-ver", dr["catalogVersionId"])
		assert.Equal(t, "primary-image", primary["imageUri"])

		// Sidecar untouched.
		sidecarRef := sidecar["codeRef"].(map[string]any)
		sidecarDR := sidecarRef["datarobot"].(map[string]any)
		assert.Equal(t, "sidecar-cat", sidecarDR["catalogId"])
		assert.Equal(t, "sidecar-ver", sidecarDR["catalogVersionId"])
	})

	t.Run("FindsPrimaryAcrossMultipleGroups", func(t *testing.T) {
		// Primary lives in containerGroups[1].containers[0]. Iteration must
		// reach it instead of bailing after the first group.
		raw := map[string]any{
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{
						"containers": []any{
							map[string]any{"primary": false},
						},
					},
					map[string]any{
						"containers": []any{
							map[string]any{"primary": true},
						},
					},
				},
			},
		}

		require.NoError(t, setPrimaryCodeRefInRawArtifact(raw, "cat-a", "ver-a"))

		primary := raw["spec"].(map[string]any)["containerGroups"].([]any)[1].(map[string]any)["containers"].([]any)[0].(map[string]any)
		require.Contains(t, primary, "codeRef")
	})

	t.Run("ErrorsWhenNoPrimary", func(t *testing.T) {
		raw := map[string]any{
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{
						"containers": []any{
							map[string]any{"primary": false},
							map[string]any{}, // no primary field at all
						},
					},
				},
			},
		}

		err := setPrimaryCodeRefInRawArtifact(raw, "cat", "ver")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no primary container")
	})

	cases := []struct {
		name    string
		raw     map[string]any
		wantSub string
	}{
		{
			"missing spec",
			map[string]any{},
			"spec missing",
		},
		{
			"spec wrong type",
			map[string]any{"spec": "not-a-map"},
			"spec missing",
		},
		{
			"empty containerGroups",
			map[string]any{"spec": map[string]any{"containerGroups": []any{}}},
			"containerGroups missing or empty",
		},
		{
			"missing containerGroups",
			map[string]any{"spec": map[string]any{}},
			"containerGroups missing or empty",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := setPrimaryCodeRefInRawArtifact(c.raw, "cat", "ver")
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantSub)
		})
	}
}
