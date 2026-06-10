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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
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
						"imageBuildConfig": {
							"codeRef": {
								"datarobot": {
									"catalogId": "cat-xyz-789",
									"catalogVersionId": "fedcba0987654321fedcba09"
								}
							},
							"dockerfile": {"source": "provided"}
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

func TestExtractCodeRef_NilImageBuildConfig(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{ImageBuildConfig: nil}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilCodeRef(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{ImageBuildConfig: &ImageBuildConfig{CodeRef: nil}}}},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact))
}

func TestExtractCodeRef_NilDatarobot(t *testing.T) {
	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{Containers: []Container{{
					ImageBuildConfig: &ImageBuildConfig{CodeRef: &CodeRef{Datarobot: nil}},
				}}},
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

// Read/write asymmetry guard: PatchArtifactCodeRef writes the primary container,
// so reads must too, even when a sidecar appears at index 0.
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
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
									CatalogID: "sidecar-cat", CatalogVersionID: "sidecar-ver",
								}},
							},
						},
						{
							Primary: &primary,
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
									CatalogID: "primary-cat", CatalogVersionID: "primary-ver",
								}},
							},
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
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
									CatalogID: "cat-2", CatalogVersionID: "ver-2",
								}},
							},
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

// Commit-to-primary semantics: a primary with no codeRef returns nil rather
// than falling through to a sidecar's stale data.
func TestExtractCodeRef_PrimaryWithoutCodeRefReturnsNil(t *testing.T) {
	primary := true
	notPrimary := false

	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							Primary: &notPrimary,
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
									CatalogID: "sidecar-cat", CatalogVersionID: "sidecar-ver",
								}},
							},
						},
						{
							Primary:          &primary,
							ImageBuildConfig: nil,
						},
					},
				},
			},
		},
	}

	assert.Nil(t, ExtractCodeRef(artifact),
		"primary container with nil imageBuildConfig must return nil, not the sidecar's coderef")
}

// Explicit primary: false is treated as a sidecar, not a candidate.
func TestExtractCodeRef_PrimaryFalseDoesNotMatch(t *testing.T) {
	notPrimary := false

	artifact := Artifact{
		Spec: Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							Primary: &notPrimary,
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{Datarobot: &DatarobotCodeRef{
									CatalogID: "first", CatalogVersionID: "v1",
								}},
							},
						},
					},
				},
			},
		},
	}

	// Fallback to [0][0] when no primary is flagged.
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
				"primary": true
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
					"primary": true,
					"port": 8080,
					"imageBuildConfig": {
						"codeRef": {"datarobot": {"catalogId": "c", "catalogVersionId": "v"}},
						"dockerfile": {
							"source": "generated",
							"executionEnvironmentId": "env-1",
							"executionEnvironmentVersionId": "env-v-1",
							"entrypoint": ["python", "agent.py"]
						}
					}
				}]
			}]
		}
	}`

	require.NoError(t, ValidateCreateRequest([]byte(spec)))
}

// TestValidateCreateRequest_BuildFromSource: imageBuildConfig present at create,
// codeRef and imageUri filled in later by the server / sync flow.
func TestValidateCreateRequest_BuildFromSource(t *testing.T) {
	spec := `{
		"name": "build-from-source",
		"spec": {
			"containerGroups": [{
				"containers": [{
					"primary": true,
					"port": 8080,
					"imageBuildConfig": {
						"dockerfile": {"source": "provided"}
					}
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

func TestGetPrimaryContainerImageURI(t *testing.T) {
	primary := true
	notPrimary := false

	t.Run("returns primary container imageUri", func(t *testing.T) {
		artifact := Artifact{
			Spec: Spec{
				ContainerGroups: []ContainerGroup{
					{Containers: []Container{
						{Primary: &notPrimary, ImageURI: "side-image:1"},
						{Primary: &primary, ImageURI: "primary-image:1"},
					}},
				},
			},
		}
		assert.Equal(t, "primary-image:1", GetPrimaryContainerImageURI(artifact))
	})

	t.Run("falls back to [0][0] when no primary marked", func(t *testing.T) {
		artifact := Artifact{
			Spec: Spec{
				ContainerGroups: []ContainerGroup{
					{Containers: []Container{
						{ImageURI: "first-image:1"},
						{ImageURI: "second-image:1"},
					}},
				},
			},
		}
		assert.Equal(t, "first-image:1", GetPrimaryContainerImageURI(artifact))
	})

	t.Run("empty when no container groups", func(t *testing.T) {
		assert.Empty(t, GetPrimaryContainerImageURI(Artifact{}))
	})

	t.Run("empty when group has no containers", func(t *testing.T) {
		artifact := Artifact{
			Spec: Spec{ContainerGroups: []ContainerGroup{{}}},
		}
		assert.Empty(t, GetPrimaryContainerImageURI(artifact))
	})
}

func TestSetPrimaryCodeRefInRawArtifact(t *testing.T) {
	t.Run("OverwritesPrimaryCodeRef_LeavesSidecarsAlone", func(t *testing.T) {
		// Sidecar at [0], primary at [1]. Only the primary repoints, and its
		// existing dockerfile config survives untouched.
		raw := map[string]any{
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{
						"containers": []any{
							map[string]any{
								"primary": false,
								"imageBuildConfig": map[string]any{
									"codeRef": map[string]any{
										"datarobot": map[string]any{
											"catalogId":        "sidecar-cat",
											"catalogVersionId": "sidecar-ver",
										},
									},
									"dockerfile": map[string]any{"source": "provided"},
								},
								"imageUri": "sidecar-image",
							},
							map[string]any{
								"primary": true,
								"imageBuildConfig": map[string]any{
									"codeRef": map[string]any{
										"datarobot": map[string]any{
											"catalogId":        "old-cat",
											"catalogVersionId": "old-ver",
										},
									},
									"dockerfile": map[string]any{
										"source":                        "generated",
										"executionEnvironmentId":        "env-1",
										"executionEnvironmentVersionId": "env-v-1",
										"entrypoint":                    []any{"python", "agent.py"},
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

		primaryIBC := primary["imageBuildConfig"].(map[string]any)
		dr := primaryIBC["codeRef"].(map[string]any)["datarobot"].(map[string]any)
		assert.Equal(t, "new-cat", dr["catalogId"])
		assert.Equal(t, "new-ver", dr["catalogVersionId"])

		df := primaryIBC["dockerfile"].(map[string]any)
		assert.Equal(t, "generated", df["source"])
		assert.Equal(t, "env-1", df["executionEnvironmentId"])
		assert.Equal(t, "env-v-1", df["executionEnvironmentVersionId"])
		assert.Equal(t, "primary-image", primary["imageUri"])

		sidecarIBC := sidecar["imageBuildConfig"].(map[string]any)
		sidecarDR := sidecarIBC["codeRef"].(map[string]any)["datarobot"].(map[string]any)
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
		require.Contains(t, primary, "imageBuildConfig")
		ibc := primary["imageBuildConfig"].(map[string]any)
		dr := ibc["codeRef"].(map[string]any)["datarobot"].(map[string]any)
		assert.Equal(t, "cat-a", dr["catalogId"])
		assert.Equal(t, "ver-a", dr["catalogVersionId"])
		assert.Equal(t, "provided", ibc["dockerfile"].(map[string]any)["source"])
	})

	t.Run("FallsBackToFirstContainerWhenNoPrimary", func(t *testing.T) {
		raw := map[string]any{
			"spec": map[string]any{
				"containerGroups": []any{
					map[string]any{
						"containers": []any{
							map[string]any{
								"imageUri": "first-image",
							},
							map[string]any{
								"imageUri": "second-image",
							},
						},
					},
				},
			},
		}

		require.NoError(t, setPrimaryCodeRefInRawArtifact(raw, "new-cat", "new-ver"))

		containers := raw["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
		first := containers[0].(map[string]any)
		second := containers[1].(map[string]any)

		ibc := first["imageBuildConfig"].(map[string]any)
		dr := ibc["codeRef"].(map[string]any)["datarobot"].(map[string]any)
		assert.Equal(t, "new-cat", dr["catalogId"])
		assert.Equal(t, "new-ver", dr["catalogVersionId"])
		assert.Equal(t, "provided", ibc["dockerfile"].(map[string]any)["source"])
		assert.NotContains(t, second, "imageBuildConfig", "second container must be untouched")
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

func TestDeleteArtifact_Success(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/artifacts/art-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteArtifact("art-1"))
}

func TestDeleteArtifact_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// '?' must arrive escaped inside the path segment, never as a query.
		assert.Equal(t, "/api/v2/artifacts/art-1%3Fforce=true/", r.URL.EscapedPath())
		assert.Empty(t, r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteArtifact("art-1?force=true"))
}

func TestDeleteArtifact_409PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail":"Cannot delete artifact referenced by 1 workload(s): wl-1. Delete the workload(s) first."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteArtifact("art-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "Delete the workload(s) first")
}
