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
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gateEnv = "DATAROBOT_CLI_FEATURE_WORKLOAD_API_LLM"

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

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv(gateEnv, "true")

	cmd := Cmd()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func names(t *testing.T) map[string]bool {
	t.Helper()

	got := map[string]bool{}
	for _, c := range Cmd().Commands() {
		got[c.Name()] = !c.Hidden
	}

	return got
}

func TestGate_ForwardsAlwaysNewCommandsOnlyWithGate(t *testing.T) {
	t.Setenv(gateEnv, "")

	off := names(t)
	assert.Equal(t, map[string]bool{"list": false, "select": false}, off,
		"without the gate only the hidden llm-gateway forwards are registered")
	assert.True(t, Cmd().Hidden, "the group stays out of help without the gate")

	t.Setenv(gateEnv, "true")

	on := names(t)
	for _, name := range []string{"deploy", "rollout", "chat", "status", "watch", "endpoint"} {
		assert.Truef(t, on[name], "%s is listed with the gate", name)
	}

	assert.False(t, on["list"], "forwards stay hidden")
	assert.False(t, Cmd().Hidden)
}

const recipeDoc = `{"id":"r1","name":"kimi","status":"available","version":3,"model":{"repoId":"moonshotai/Kimi-K3.1-Pro"}}`

func TestDeploy_DryRunPrintsRequest(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "a dry run only reads the recipe")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, recipeDoc)
	}))

	out, err := run(t, "deploy", "--recipe", "r1", "--name", "kimi-prod", "--bundle", "gpu.b200.x8", "--dry-run")
	require.NoError(t, err)

	var body map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &body))
	assert.Equal(t, "kimi-prod", body["name"])
	assert.Contains(t, out, `"repoId": "moonshotai/Kimi-K3.1-Pro"`, "the model defaults to the recipe's")
	assert.Contains(t, out, `"servedName": "kimi-k3.1-pro"`)
}

func TestDeploy_RefusesInvalidRecipe(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "nothing is created from an invalid recipe")
		writeJSON(w, http.StatusOK, map[string]any{"id": "r1", "status": "invalid", "version": 2})
	}))

	_, err := run(t, "deploy", "--recipe", "r1", "--name", "n", "--bundle", "b")
	require.ErrorContains(t, err, "is invalid")
}

func TestDeploy_Creates(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, recipeDoc)

			return
		}

		assert.Equal(t, "/api/v2/workloads/", r.URL.Path)
		writeJSON(w, http.StatusCreated, map[string]any{"id": "w1", "name": "kimi-prod", "status": "submitted", "artifactId": "a1"})
	}))

	out, err := run(t, "deploy", "--recipe", "r1", "--name", "kimi-prod", "--bundle", "gpu.b200.x8")
	require.NoError(t, err)
	assert.Contains(t, out, "Workload w1 (kimi-prod) created from recipe r1.")
	assert.Contains(t, out, "dr llm chat w1")
}

func TestRollout_DryRunSendsCleanRuntime(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/workloads/w1/":
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "w1", "type": "llm", "artifactId": "a1",
				"runtime": map[string]any{
					"recipe":         map[string]any{"id": "r1"},
					"resolvedRecipe": map[string]any{"id": "r1", "version": 2},
					"workers":        map[string]any{"resourceBundles": []string{"gpu.b200.x8"}, "replicaCount": json.Number("9007199254740993")},
				},
			})
		case "/api/v2/llmRecipes/r1":
			assert.Equal(t, "3", r.URL.Query().Get("version"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, recipeDoc)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))

	out, err := run(t, "rollout", "w1", "--recipe-version", "3", "--dry-run")
	require.NoError(t, err)

	var body struct {
		ArtifactID string         `json:"artifactId"`
		Strategy   string         `json:"strategy"`
		Runtime    map[string]any `json:"runtime"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &body))
	assert.Equal(t, "a1", body.ArtifactID)
	assert.Equal(t, "rolling", body.Strategy)
	assert.NotContains(t, body.Runtime, "resolvedRecipe")
	assert.Equal(t, map[string]any{"id": "r1", "mode": "strict", "version": 3.0}, body.Runtime["recipe"])
	assert.Contains(t, out, "9007199254740993", "runtime integers are sent back exactly")
}

func TestRollout_NeedsATarget(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request without a target")
	}))

	_, err := run(t, "rollout", "w1")
	require.ErrorContains(t, err, "--recipe-version N, --latest, or --recipe")

	_, err = run(t, "rollout", "w1", "--recipe-version", "2", "--latest")
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestChat_EndToEnd(t *testing.T) {
	var base string

	base = serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/workloads/w1/":
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "w1", "type": "llm", "status": "running", "artifactId": "a1",
				"endpoint": base + "/api/v2/endpoints/workloads/w1/",
			})
		case "/api/v2/artifacts/a1/":
			writeJSON(w, http.StatusOK, map[string]any{"type": "llm", "spec": map[string]any{"model": map[string]any{"servedName": "kimi"}}})
		case "/api/v2/endpoints/workloads/w1/v1/chat/completions":
			assert.Equal(t, "kimi", r.Header.Get("x-ai-eg-model"))
			writeJSON(w, http.StatusOK, map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "Hello!"}}}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))

	out, err := run(t, "chat", "w1", "Say hello")
	require.NoError(t, err)
	assert.Equal(t, "Hello!\n", out)
}

func TestChat_ExplainsRefusalWithServedModels(t *testing.T) {
	var base string

	base = serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/workloads/w1/":
			writeJSON(w, http.StatusOK, map[string]any{"id": "w1", "type": "llm", "artifactId": "a1", "endpoint": base + "/api/v2/endpoints/workloads/w1/"})
		case "/api/v2/artifacts/a1/":
			writeJSON(w, http.StatusOK, map[string]any{"type": "llm", "spec": map[string]any{"model": map[string]any{"servedName": "embed"}}})
		case "/api/v2/endpoints/workloads/w1/v1/chat/completions":
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "no chat template"})
		case "/api/v2/endpoints/workloads/w1/v1/models":
			writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{{"id": "embed"}}})
		}
	}))

	_, err := run(t, "chat", "w1", "hi")
	require.ErrorContains(t, err, "may have no chat template")
}
