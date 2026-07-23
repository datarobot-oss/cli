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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryEnvironmentVars(t *testing.T) {
	primary := true
	notPrimary := false

	t.Run("returns primary container's vars", func(t *testing.T) {
		artifact := Artifact{
			Spec: Spec{
				ContainerGroups: []ContainerGroup{
					{Containers: []Container{
						{Primary: &notPrimary, EnvironmentVars: []EnvironmentVar{{Name: "SIDE"}}},
						{Primary: &primary, EnvironmentVars: []EnvironmentVar{{Name: "MAIN"}}},
					}},
				},
			},
		}
		vars := PrimaryEnvironmentVars(artifact)
		require.Len(t, vars, 1)
		assert.Equal(t, "MAIN", vars[0].Name)
	})

	t.Run("falls back to [0][0] when no primary marked", func(t *testing.T) {
		artifact := Artifact{
			Spec: Spec{
				ContainerGroups: []ContainerGroup{
					{Containers: []Container{
						{EnvironmentVars: []EnvironmentVar{{Name: "FIRST"}}},
						{EnvironmentVars: []EnvironmentVar{{Name: "SECOND"}}},
					}},
				},
			},
		}
		vars := PrimaryEnvironmentVars(artifact)
		require.Len(t, vars, 1)
		assert.Equal(t, "FIRST", vars[0].Name)
	})

	t.Run("nil when no container groups", func(t *testing.T) {
		assert.Nil(t, PrimaryEnvironmentVars(Artifact{}))
	})
}

func TestUpsertByName(t *testing.T) {
	existing := []EnvironmentVar{
		{Name: "A", Value: "1"},
		{Name: "B", Value: "2"},
	}

	t.Run("replaces an existing entry in place", func(t *testing.T) {
		result := upsertByName(existing, []EnvironmentVar{{Name: "A", Value: "updated"}})
		require.Len(t, result, 2)
		assert.Equal(t, "updated", result[0].Value)
		assert.Equal(t, "B", result[1].Name)
	})

	t.Run("appends a new entry at the end", func(t *testing.T) {
		result := upsertByName(existing, []EnvironmentVar{{Name: "C", Value: "3"}})
		require.Len(t, result, 3)
		assert.Equal(t, "C", result[2].Name)
	})

	t.Run("does not mutate the input slice", func(t *testing.T) {
		_ = upsertByName(existing, []EnvironmentVar{{Name: "A", Value: "mutated"}})
		assert.Equal(t, "1", existing[0].Value, "upsertByName must copy before writing")
	})
}

func TestPresentEnvironmentVarNames(t *testing.T) {
	vars := []EnvironmentVar{{Name: "A"}, {Name: "B"}}

	assert.Equal(t, []string{"A"}, PresentEnvironmentVarNames(vars, []string{"A", "NOPE"}))
	assert.Empty(t, PresentEnvironmentVarNames(vars, []string{"NOPE"}))
	assert.Empty(t, PresentEnvironmentVarNames(nil, []string{"A"}))
}

func TestRemoveByName(t *testing.T) {
	existing := []EnvironmentVar{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
	}

	remaining, removed := removeByName(existing, []string{"B", "NOPE"})

	require.Len(t, remaining, 2)
	assert.Equal(t, "A", remaining[0].Name)
	assert.Equal(t, "C", remaining[1].Name)
	assert.Equal(t, []string{"B"}, removed, "only names actually present are reported as removed")
}

func TestEnvironmentVarsRawRoundTrip(t *testing.T) {
	raw := []any{
		map[string]any{"name": "A", "value": "1"},
		map[string]any{"source": "dr-credential", "name": "B", "drCredentialId": "cred-1", "key": "apiToken"},
	}

	vars, err := toEnvironmentVars(raw)
	require.NoError(t, err)
	require.Len(t, vars, 2)
	assert.Equal(t, "A", vars[0].Name)
	assert.Equal(t, "1", vars[0].Value)
	assert.Equal(t, EnvironmentVarSourceDRCredential, vars[1].Source)
	assert.Equal(t, "cred-1", vars[1].DRCredentialID)
	assert.Equal(t, "apiToken", vars[1].Key)

	back, err := toRawEnvironmentVars(vars)
	require.NoError(t, err)
	require.Len(t, back, 2)

	first, ok := back[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "A", first["name"])
}

func TestPrimaryContainerRaw(t *testing.T) {
	t.Run("finds container flagged primary across groups", func(t *testing.T) {
		groups := []any{
			map[string]any{"containers": []any{map[string]any{"primary": false, "name": "side"}}},
			map[string]any{"containers": []any{map[string]any{"primary": true, "name": "main"}}},
		}

		container, err := primaryContainerRaw(groups)
		require.NoError(t, err)
		assert.Equal(t, "main", container["name"])
	})

	t.Run("falls back to [0][0] when no primary marked", func(t *testing.T) {
		groups := []any{
			map[string]any{"containers": []any{
				map[string]any{"name": "first"},
				map[string]any{"name": "second"},
			}},
		}

		container, err := primaryContainerRaw(groups)
		require.NoError(t, err)
		assert.Equal(t, "first", container["name"])
	})

	t.Run("errors on empty containers in the fallback group", func(t *testing.T) {
		groups := []any{map[string]any{"containers": []any{}}}
		_, err := primaryContainerRaw(groups)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "containers missing or empty")
	})
}

func TestCloneEnvName(t *testing.T) {
	name := cloneEnvName("my-artifact")
	assert.True(t, strings.HasPrefix(name, "my-artifact-env-"))
	assert.Greater(t, len(name), len("my-artifact-env-"), "must include a timestamp suffix")
}

// artifactHandler builds an httptest handler serving a single artifact
// document at /api/v2/artifacts/{id}/ for GET, and accepting PATCH by
// merging the request's "spec" into the stored document -- enough to
// exercise ApplyEnvironmentVars's GET-mutate-PATCH cycle without a real
// server.
func artifactHandler(t *testing.T, id string, doc map[string]any, patches *[]map[string]any) http.HandlerFunc {
	t.Helper()

	path := "/api/v2/artifacts/" + id + "/"

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)

			return
		}

		switch r.Method {
		case http.MethodGet:
			assert.NoError(t, json.NewEncoder(w).Encode(doc))
		case http.MethodPatch:
			var body map[string]any

			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

			*patches = append(*patches, body)

			if spec, ok := body["spec"]; ok {
				doc["spec"] = spec
			}

			if status, ok := body["status"]; ok {
				doc["status"] = status
			}

			assert.NoError(t, json.NewEncoder(w).Encode(doc))
		default:
			http.Error(w, "unexpected method "+r.Method, http.StatusMethodNotAllowed)
		}
	}
}

func draftArtifactDoc(id string) map[string]any {
	return map[string]any{
		"id":     id,
		"name":   "my-artifact",
		"status": "draft",
		"spec": map[string]any{
			"containerGroups": []any{
				map[string]any{
					"containers": []any{
						map[string]any{
							"name":            "main",
							"primary":         true,
							"environmentVars": []any{map[string]any{"name": "EXISTING", "value": "1"}},
						},
					},
				},
			},
		},
	}
}

func TestApplyEnvironmentVars_DraftPatchesInPlace(t *testing.T) {
	installSkipAuth(t)

	doc := draftArtifactDoc("art-1")

	var patches []map[string]any

	srv := httptest.NewServer(artifactHandler(t, "art-1", doc, &patches))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	targetID, needsLock, err := ApplyEnvironmentVars("art-1", func(current []any) ([]any, error) {
		return append(current, map[string]any{"name": "NEW", "value": "2"}), nil
	})
	require.NoError(t, err)
	assert.Equal(t, "art-1", targetID, "draft edits patch the same artifact in place")
	assert.False(t, needsLock, "a draft-running workload replaces onto another draft; no lock needed")
	require.Len(t, patches, 1, "exactly one PATCH for a draft artifact")

	spec := patches[0]["spec"].(map[string]any)
	containers := spec["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
	envVars := containers[0].(map[string]any)["environmentVars"].([]any)
	require.Len(t, envVars, 2)
	assert.Equal(t, "NEW", envVars[1].(map[string]any)["name"])
}

func TestApplyEnvironmentVars_LockedClonesAndLeavesDraftUnlocked(t *testing.T) {
	installSkipAuth(t)

	originalDoc := map[string]any{
		"id":     "art-1",
		"name":   "my-artifact",
		"status": "locked",
		"spec":   draftArtifactDoc("art-1")["spec"],
	}
	cloneDoc := draftArtifactDoc("art-2")

	var clonePatches []map[string]any

	var cloneRequestBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "a locked artifact is only ever read here, never patched")
		assert.NoError(t, json.NewEncoder(w).Encode(originalDoc))
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/clone/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&cloneRequestBody))
		assert.NoError(t, json.NewEncoder(w).Encode(cloneDoc))
	})
	mux.HandleFunc("/api/v2/artifacts/art-2/", artifactHandler(t, "art-2", cloneDoc, &clonePatches))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	installEndpoint(t, srv.URL)

	targetID, needsLock, err := ApplyEnvironmentVars("art-1", func(current []any) ([]any, error) {
		return append(current, map[string]any{"name": "NEW", "value": "2"}), nil
	})
	require.NoError(t, err)
	assert.Equal(t, "art-2", targetID, "a locked source produces a different (cloned) target artifact")
	assert.True(t, needsLock, "the clone must be locked before it can replace a locked artifact, but that is the caller's job")
	assert.Contains(t, cloneRequestBody["name"], "my-artifact-env-")
	assert.Len(t, clonePatches, 1, "the clone is patched exactly once")
	assert.NotEqual(t, "locked", cloneDoc["status"], "ApplyEnvironmentVars must not lock the clone itself")
}

// TestApplyEnvironmentVars_PatchFailureAfterCloneNamesTheOrphan guards
// against silently losing track of a clone: if the PATCH on a freshly
// cloned artifact fails, the clone still exists (as a harmless, deletable
// draft) -- the error must name it so the caller isn't left hunting for it.
func TestApplyEnvironmentVars_PatchFailureAfterCloneNamesTheOrphan(t *testing.T) {
	installSkipAuth(t)

	originalDoc := map[string]any{
		"id":     "art-1",
		"name":   "my-artifact",
		"status": "locked",
		"spec":   draftArtifactDoc("art-1")["spec"],
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewEncoder(w).Encode(originalDoc))
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/clone/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"art-2","name":"my-artifact-env-123","status":"draft"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"detail":"boom"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ApplyEnvironmentVars("art-1", func(current []any) ([]any, error) {
		return current, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-2", "the orphaned clone's id must be named in the error")
	assert.Contains(t, err.Error(), "art-1", "the source locked artifact should also be named for context")
}

func TestUpsertEnvironmentVars_MergesByName(t *testing.T) {
	installSkipAuth(t)

	doc := draftArtifactDoc("art-1")

	var patches []map[string]any

	srv := httptest.NewServer(artifactHandler(t, "art-1", doc, &patches))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	targetID, needsLock, err := UpsertEnvironmentVars("art-1", []EnvironmentVar{
		{Name: "EXISTING", Value: "updated"},
		{Name: "NEW", Value: "added"},
	})
	require.NoError(t, err)
	assert.Equal(t, "art-1", targetID)
	assert.False(t, needsLock)

	spec := patches[0]["spec"].(map[string]any)
	containers := spec["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
	envVars := containers[0].(map[string]any)["environmentVars"].([]any)
	require.Len(t, envVars, 2)
	assert.Equal(t, "updated", envVars[0].(map[string]any)["value"])
	assert.Equal(t, "NEW", envVars[1].(map[string]any)["name"])
}

func TestRemoveEnvironmentVars_ReportsRemovedNames(t *testing.T) {
	installSkipAuth(t)

	doc := draftArtifactDoc("art-1")

	var patches []map[string]any

	srv := httptest.NewServer(artifactHandler(t, "art-1", doc, &patches))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	targetID, needsLock, removed, err := RemoveEnvironmentVars("art-1", []string{"EXISTING", "NOPE"})
	require.NoError(t, err)
	assert.Equal(t, "art-1", targetID)
	assert.False(t, needsLock)
	assert.Equal(t, []string{"EXISTING"}, removed)

	spec := patches[0]["spec"].(map[string]any)
	containers := spec["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)
	envVars := containers[0].(map[string]any)["environmentVars"].([]any)
	assert.Empty(t, envVars)
}

func TestCreateArtifactClone_PostsNameAndDecodes(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/artifacts/art-1/clone/", r.URL.Path)

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "art-1-env-123", body["name"])

		fmt.Fprint(w, `{"id":"art-2","name":"art-1-env-123","status":"draft"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	clone, err := CreateArtifactClone("art-1", "art-1-env-123")
	require.NoError(t, err)
	assert.Equal(t, "art-2", clone.ID)
	assert.Equal(t, "draft", clone.Status)
}

func TestResolveWorkloadArtifact(t *testing.T) {
	installSkipAuth(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","artifactId":"art-1"}`)
	})
	mux.HandleFunc("/api/v2/artifacts/art-1/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"art-1","status":"draft"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	installEndpoint(t, srv.URL)

	wl, artifact, err := ResolveWorkloadArtifact("wl-1")
	require.NoError(t, err)
	assert.Equal(t, "wl-1", wl.ID)
	assert.Equal(t, "art-1", artifact.ID)
}
