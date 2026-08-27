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
	"strings"
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

func TestLockArtifact_SendsStatusLocked(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/artifacts/art-1/", r.URL.Path)

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]any{"status": "locked"}, body)

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"art-1","name":"my-agent","status":"locked"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	artifact, err := LockArtifact("art-1")
	require.NoError(t, err)
	assert.Equal(t, "art-1", artifact.ID)
	assert.Equal(t, "locked", artifact.Status)
}

func TestLockArtifact_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// '?' must arrive escaped inside the path segment, never as a query.
		assert.Equal(t, "/api/v2/artifacts/art-1%3Fforce=true/", r.URL.EscapedPath())
		assert.Empty(t, r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"art-1","status":"locked"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := LockArtifact("art-1?force=true")
	require.NoError(t, err)
}

func TestLockArtifact_422PropagatesDetail(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail":"Cannot lock artifact: imageBuildConfig is set but imageUri is not populated. Trigger and complete an image build before locking."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := LockArtifact("art-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "complete an image build before locking")
}

func TestLockArtifact_403AlreadyLockedPropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"detail":"Cannot update artifact: artifact is locked"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := LockArtifact("art-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "artifact is locked")
}

func TestPrimaryContainerName(t *testing.T) {
	primary := true

	t.Run("returns the name of the primary-flagged container", func(t *testing.T) {
		art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{
			{Containers: []Container{{Name: "sidecar"}, {Name: "main", Primary: &primary}}},
		}}}
		assert.Equal(t, "main", PrimaryContainerName(art))
	})

	t.Run("falls back to the first container when none is flagged primary", func(t *testing.T) {
		art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{
			{Containers: []Container{{Name: "only"}}},
		}}}
		assert.Equal(t, "only", PrimaryContainerName(art))
	})

	t.Run("falls back to \"primary\" when the artifact has no named container", func(t *testing.T) {
		assert.Equal(t, "primary", PrimaryContainerName(Artifact{}))
	})

	t.Run("skips a primary container that has no name", func(t *testing.T) {
		art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{
			{Containers: []Container{{Primary: &primary}}},
		}}}
		assert.Equal(t, "primary", PrimaryContainerName(art))
	})
}

func TestPrimaryEnvironmentVars(t *testing.T) {
	primary := true

	plain := EnvironmentVar{Name: "LOG_LEVEL", Value: "debug"}
	secret := EnvironmentVar{
		Source:         EnvironmentVarSourceDRCredential,
		Name:           "OPENAI_API_KEY",
		DRCredentialID: "66f0000000000000000000aa",
		Key:            "apiToken",
	}

	t.Run("reads the primary-flagged container", func(t *testing.T) {
		art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{{Containers: []Container{
			{Name: "sidecar", EnvironmentVars: []EnvironmentVar{{Name: "WRONG"}}},
			{Name: "main", Primary: &primary, EnvironmentVars: []EnvironmentVar{plain, secret}},
		}}}}}
		assert.Equal(t, []EnvironmentVar{plain, secret}, PrimaryEnvironmentVars(art))
	})

	t.Run("falls back to the first container when none is flagged primary", func(t *testing.T) {
		art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{
			{Containers: []Container{{Name: "only", EnvironmentVars: []EnvironmentVar{plain}}}},
		}}}
		assert.Equal(t, []EnvironmentVar{plain}, PrimaryEnvironmentVars(art))
	})

	t.Run("returns nil for an artifact with no groups or no containers", func(t *testing.T) {
		assert.Nil(t, PrimaryEnvironmentVars(Artifact{}))
		assert.Nil(t, PrimaryEnvironmentVars(Artifact{Spec: Spec{
			ContainerGroups: []ContainerGroup{{Containers: []Container{}}},
		}}))
	})
}

// A credential-backed var must serialize to the reference shape and never
// carry a value; a plain var must omit the credential fields and the source
// discriminator, which the server defaults.
func TestEnvironmentVar_MarshalShapes(t *testing.T) {
	secret, err := json.Marshal(EnvironmentVar{
		Source:         EnvironmentVarSourceDRCredential,
		Name:           "OPENAI_API_KEY",
		DRCredentialID: "66f0000000000000000000aa",
		Key:            "apiToken",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"source": "dr-credential",
		"name": "OPENAI_API_KEY",
		"drCredentialId": "66f0000000000000000000aa",
		"key": "apiToken"
	}`, string(secret))

	plain, err := json.Marshal(EnvironmentVar{Name: "LOG_LEVEL", Value: "debug"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name": "LOG_LEVEL", "value": "debug"}`, string(plain))
}

// Container.Name and EnvironmentVars must survive a round trip through the
// server document shape, since the plan renderer and runtime-spec builder
// both read them off a fetched artifact.
func TestContainer_ParsesNameAndEnvironmentVars(t *testing.T) {
	var art Artifact

	require.NoError(t, json.Unmarshal([]byte(`{
		"id": "art-1",
		"spec": {"containerGroups": [{"containers": [{
			"name": "primary",
			"primary": true,
			"imageUri": "example/app:1",
			"environmentVars": [
				{"name": "LOG_LEVEL", "value": "debug"},
				{"source": "dr-credential", "name": "TOKEN", "drCredentialId": "66f", "key": "apiToken"}
			]
		}]}]}
	}`), &art))

	assert.Equal(t, "primary", PrimaryContainerName(art))

	vars := PrimaryEnvironmentVars(art)
	require.Len(t, vars, 2)
	assert.Equal(t, "debug", vars[0].Value)
	assert.Empty(t, vars[0].Source, "a plain var carries no source discriminator")
	assert.Equal(t, EnvironmentVarSourceDRCredential, vars[1].Source)
	assert.Equal(t, "apiToken", vars[1].Key)
	assert.Empty(t, vars[1].Value, "a credential-backed var never carries a literal value")
}

// The four Primary* helpers must always describe the same container. Before
// they shared a traversal, PrimaryContainerName additionally required a
// non-empty name, so an unnamed primary made it fall through and report a
// sidecar's name while the others read the primary.
func TestPrimaryHelpers_AgreeOnUnnamedPrimary(t *testing.T) {
	primary := true

	art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{{Containers: []Container{
		{
			Name:            "sidecar",
			ImageURI:        "sidecar:1",
			EnvironmentVars: []EnvironmentVar{{Name: "WRONG"}},
		},
		{
			Name:            "",
			Primary:         &primary,
			ImageURI:        "app:1",
			EnvironmentVars: []EnvironmentVar{{Name: "RIGHT"}},
		},
	}}}}}

	assert.Equal(t, "app:1", GetPrimaryContainerImageURI(art))
	require.Len(t, PrimaryEnvironmentVars(art), 1)
	assert.Equal(t, "RIGHT", PrimaryEnvironmentVars(art)[0].Name)
	assert.Equal(t, "primary", PrimaryContainerName(art),
		"an unnamed primary falls back to the CLI's own convention, never to a sidecar's name")
	assert.NotEqual(t, "sidecar", PrimaryContainerName(art))
}

// A primary container in a later group must win over an earlier group's
// unflagged container, for every helper.
func TestPrimaryHelpers_AgreeAcrossGroups(t *testing.T) {
	primary := true

	art := Artifact{Spec: Spec{ContainerGroups: []ContainerGroup{
		{Containers: []Container{{Name: "first", ImageURI: "first:1"}}},
		{Containers: []Container{{Name: "real", Primary: &primary, ImageURI: "real:1"}}},
	}}}

	assert.Equal(t, "real", PrimaryContainerName(art))
	assert.Equal(t, "real:1", GetPrimaryContainerImageURI(art))
}

// serverArtifactDoc builds a minimal artifact JSON doc for list-page tests.
func serverArtifactDoc(id, name, status string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"status":%q}`, id, name, status)
}

// artifactListPage wraps docs in the artifact list pagination envelope, mirroring
// workloadListPage so the artifact list tests can assert next-link following.
func artifactListPage(next string, docs ...string) string {
	nextJSON := "null"
	if next != "" {
		nextJSON = fmt.Sprintf("%q", next)
	}

	return fmt.Sprintf(
		`{"data": [%s], "count": %d, "totalCount": %d, "next": %s, "previous": null}`,
		strings.Join(docs, ","), len(docs), len(docs), nextJSON,
	)
}

func TestListArtifacts_RejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		_, err := ListArtifacts(limit, 0, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	}
}

func TestListArtifacts_RejectsNegativeOffset(t *testing.T) {
	for _, offset := range []int{-1} {
		_, err := ListArtifacts(1, offset, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be non-negative")
	}
}

func TestListArtifacts_ClampsPageSizeToServerMax(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server rejects limit > 100 with a 422, so the page size must
		// be clamped even when --limit asks for more.
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		fmt.Fprint(w, artifactListPage("", serverArtifactDoc("art-1", "a", "draft")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := ListArtifacts(250, 0, "")
	require.NoError(t, err)
}
