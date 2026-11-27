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

package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveAPI points the CLI at an httptest server with auth stubbed out, as
// internal/workload's helper of the same name does. It returns the URL.
func serveAPI(t *testing.T, handler http.Handler) string {
	t.Helper()

	prevSkip := viperx.GetBool(config.SkipAuthKey)
	prevTok := viperx.GetString(config.DataRobotAPIKey)
	prevURL := viperx.GetString(config.DataRobotURL)

	viperx.Set(config.SkipAuthKey, true)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	srv := httptest.NewServer(handler)

	t.Cleanup(srv.Close)

	viperx.Set(config.DataRobotURL, srv.URL)

	t.Cleanup(func() {
		viperx.Set(config.SkipAuthKey, prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevTok)
		viperx.Set(config.DataRobotURL, prevURL)
	})

	return srv.URL
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestListRecipes_FiltersAndFollowsNext(t *testing.T) {
	var base string

	var queries []string

	base = serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)

		if r.URL.Query().Get("offset") == "0" {
			writeJSON(w, http.StatusOK, map[string]any{
				"data": []map[string]any{{"id": "a", "name": "one"}},
				"next": base + "/api/v2/llmRecipes/?offset=1&limit=50",
			})

			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{{"id": "b", "name": "two"}}})
	}))

	recipes, err := ListRecipes(ListOptions{Kind: "dynamo", GPUSku: "B200", IDs: []string{"x", "y"}, Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, []string{recipes[0].ID, recipes[1].ID})
	require.NotEmpty(t, queries)
	assert.Contains(t, queries[0], "kind=dynamo")
	assert.Contains(t, queries[0], "gpuSku=B200")
	assert.Contains(t, queries[0], "ids=x&ids=y")
}

func TestListRecipes_StopsAtLimit(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"id": "a"}, {"id": "b"}, {"id": "c"}},
			"next": "http://elsewhere.example/should-not-be-followed",
		})
	}))

	recipes, err := ListRecipes(ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, recipes, 2)
}

func TestListRecipes_RefusesNextOnAnotherHost(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"id": "a"}},
			"next": "http://elsewhere.example/api/v2/llmRecipes/?offset=1",
		})
	}))

	_, err := ListRecipes(ListOptions{Limit: 10})
	require.Error(t, err)
}

func TestGetRecipe_PassesVersion(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/llmRecipes/abc", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("version"))
		writeJSON(w, http.StatusOK, map[string]any{"id": "abc", "version": 2, "model": map[string]any{"repoId": "org/m"}})
	}))

	r, err := GetRecipe("abc", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, r.Version)
	assert.Equal(t, "org/m", r.ModelName())
}

func TestGateErrors_NameTheGate(t *testing.T) {
	cases := map[string]string{
		"LLM artifacts are disabled on this cluster":     ClusterDisabledRemedy,
		"Recipes are not available for the current user": UserFlagRemedy,
		// workload-api's Workload Storage refusals, as a model weight mount
		// meets them; the first also contains the LLM serving phrase.
		"Workload Storage is disabled on this cluster":  StorageClusterDisabledRemedy,
		"Workload Storage is not enabled for this user": StorageUserFlagRemedy,
	}

	for detail, remedy := range cases {
		t.Run(detail, func(t *testing.T) {
			serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusForbidden, map[string]any{"detail": detail})
			}))

			_, err := ListRecipes(ListOptions{Limit: 1})
			require.Error(t, err)

			var gate *GateError

			require.ErrorAs(t, err, &gate)
			assert.Equal(t, remedy, gate.Remedy)

			var httpErr *drapi.HTTPError

			require.ErrorAs(t, err, &httpErr, "the HTTPError chain must survive")
			assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}
}

func TestGateErrors_OtherForbiddenUntouched(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"detail": "The recipe belongs to another user"})
	}))

	err := DeleteRecipe("abc")
	require.Error(t, err)

	var gate *GateError

	assert.NotErrorAs(t, err, &gate)
	assert.Contains(t, err.Error(), "belongs to another user")
}

func TestCreateRecipe_SendsBody(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		body, _ := io.ReadAll(r.Body)

		var got map[string]any

		assert.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "qwen", got["name"])
		assert.NotContains(t, got, "descriptor", "an empty descriptor is left for the server to infer")
		writeJSON(w, http.StatusCreated, map[string]any{"id": "new", "name": "qwen", "status": "available"})
	}))

	r, err := CreateRecipe(CreateRequest{Name: "qwen", Payload: json.RawMessage(`{"format":"vllm.recipe/v1","recipe":{}}`)})
	require.NoError(t, err)
	assert.Equal(t, "new", r.ID)
}

func TestPatchRequest_Validate(t *testing.T) {
	name := "n"
	v := 3
	payload := json.RawMessage(`{"format":"vllm.recipe/v1","recipe":{}}`)

	assert.NoError(t, PatchRequest{Name: &name}.Validate())
	assert.NoError(t, PatchRequest{Payload: payload, IfVersion: &v}.Validate())
	require.ErrorContains(t, PatchRequest{Payload: payload}.Validate(), "--if-version")
	require.ErrorContains(t, PatchRequest{Descriptor: json.RawMessage(`{}`)}.Validate(), "descriptor")
	require.ErrorContains(t, PatchRequest{}.Validate(), "nothing to change")
}

func TestReadRecipeFile(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

		return p
	}

	full := write("full.yaml", "name: q\npayload:\n  format: vllm.recipe/v1\n  recipe:\n    hf_id: Qwen/Qwen3-32B\n")
	f, err := ReadRecipeFile(full)
	require.NoError(t, err)
	assert.Equal(t, "q", f.Name)
	assert.Equal(t, FormatVLLMRecipe, PayloadFormat(f.Payload))

	bare := write("bare.yaml", "format: dynamo.dgd/v1beta1\nobjects:\n  - kind: DynamoGraphDeployment\n")
	f, err = ReadRecipeFile(bare)
	require.NoError(t, err)
	assert.Empty(t, f.Name)
	assert.Equal(t, FormatDynamoDGD, PayloadFormat(f.Payload))

	_, err = ReadRecipeFile(write("noobjects.yaml", "format: dynamo.dgd/v1beta1\nobjects: []\n"))
	require.ErrorContains(t, err, "payload.objects")

	_, err = ReadRecipeFile(write("badrecipe.yaml", "format: vllm.recipe/v1\nrecipe: nope\n"))
	require.ErrorContains(t, err, "payload.recipe")

	_, err = ReadRecipeFile(write("unknown.yaml", "format: other/v1\n"))
	require.ErrorContains(t, err, "payload.format")

	_, err = ReadRecipeFile(write("nopayload.yaml", "name: q\n"))
	require.ErrorContains(t, err, "no payload")
}

func TestRecipeDeployments_MatchesResolvedThenRequested(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "llm", r.URL.Query().Get("type"))
		writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{
			{"id": "w1", "runtime": map[string]any{"resolvedRecipe": map[string]any{"id": "r1", "version": 4}}},
			{"id": "w2", "runtime": map[string]any{"recipe": map[string]any{"id": "r1", "version": 2}}},
			{"id": "w3", "runtime": map[string]any{"recipe": map[string]any{"id": "r1"}}},
			{"id": "w4", "runtime": map[string]any{"recipe": map[string]any{"id": "other"}}},
			{"id": "w5", "runtime": map[string]any{}},
		}})
	}))

	deps, err := RecipeDeployments("r1", nil, 10)
	require.NoError(t, err)
	require.Len(t, deps, 3)
	assert.Equal(t, []string{"4", "2", "latest"}, []string{deps[0].RecipeVersion(), deps[1].RecipeVersion(), deps[2].RecipeVersion()})
}

func TestRenderRecipe_ShowsDiagnostics(t *testing.T) {
	r := Recipe{
		ID: "abc", Name: "q", Status: StatusInvalid, Version: 1,
		Provenance: json.RawMessage(`{"diagnostics":[{"severity":"error","code":"image","message":"not the runtime image","path":"objects[0]"}]}`),
	}

	var buf bytes.Buffer

	require.NoError(t, RenderRecipe(&buf, outputformat.OutputFormatText, r))
	assert.Contains(t, buf.String(), "error image at objects[0]: not the runtime image")

	buf.Reset()
	require.NoError(t, RenderRecipe(&buf, outputformat.OutputFormatJSON, r))

	var env map[string]json.RawMessage

	require.NoError(t, json.Unmarshal(buf.Bytes(), &env), "JSON output must be pure JSON: "+buf.String())
	assert.Contains(t, env, "recipe")
}

func TestRenderPayload_YAML(t *testing.T) {
	var buf bytes.Buffer

	r := Recipe{ID: "abc", Payload: json.RawMessage(`{"format":"vllm.recipe/v1","recipe":{"hf_id":"Qwen/Qwen3-32B","base_args":["--max-model-len","8192"]}}`)}
	require.NoError(t, RenderPayload(&buf, outputformat.OutputFormatText, r))
	assert.Equal(t, "format: vllm.recipe/v1\nrecipe:\n  hf_id: Qwen/Qwen3-32B\n  base_args:\n    - --max-model-len\n    - \"8192\"\n", buf.String(),
		"block style, two-space indent, and a numeric-looking string stays quoted")
}

func TestListRecipes_RefusesARepeatedPage(t *testing.T) {
	var base string

	base = serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"id": "a"}},
			"next": base + "/api/v2/llmRecipes/?offset=1",
		})
	}))

	_, err := ListRecipes(ListOptions{Limit: 50})
	require.ErrorContains(t, err, "already read")
}

func TestRecipeDeployments_RefusesAnEmptyPageWithNext(t *testing.T) {
	var base string

	base = serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{},
			"next": base + "/api/v2/workloads/?offset=" + r.URL.Query().Get("offset") + "0",
		})
	}))

	_, err := RecipeDeployments("r1", nil, 10)
	require.ErrorContains(t, err, "empty page")
}

const bigIntPayload = `{"format":"vllm.recipe/v1","recipe":{"hf_id":"Qwen/Qwen3-32B","seed":9007199254740993,"tag":"123","ratio":0.9}}`

func TestRenderPayload_KeepsLargeIntegersExact(t *testing.T) {
	r := Recipe{ID: "abc", Payload: json.RawMessage(bigIntPayload)}

	var js bytes.Buffer

	require.NoError(t, RenderPayload(&js, outputformat.OutputFormatJSON, r))
	assert.Contains(t, js.String(), "9007199254740993")

	var y bytes.Buffer

	require.NoError(t, RenderPayload(&y, outputformat.OutputFormatText, r))
	assert.Contains(t, y.String(), "seed: 9007199254740993")
	assert.Contains(t, y.String(), `tag: "123"`, "a numeric-looking string stays a string")
	assert.NotContains(t, y.String(), "{", "block style, not JSON flow style")

	// The YAML re-reads to the same payload: the get, edit, patch loop.
	p := filepath.Join(t.TempDir(), "recipe.yaml")
	require.NoError(t, os.WriteFile(p, y.Bytes(), 0o600))

	f, err := ReadRecipeFile(p)
	require.NoError(t, err)
	assert.JSONEq(t, bigIntPayload, string(f.Payload))
}
