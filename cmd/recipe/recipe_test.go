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

package recipe

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveAPI(t *testing.T, handler http.Handler) {
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
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := Cmd()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func recipeFile(t *testing.T, content string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "recipe.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))

	return p
}

const barePayload = "format: vllm.recipe/v1\nrecipe:\n  hf_id: Qwen/Qwen3-32B\n"

func TestCreate_BarePayloadNeedsName(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request is sent without a name")
	}))

	_, err := run(t, "create", recipeFile(t, barePayload))
	require.ErrorContains(t, err, "needs a name")
}

func TestCreate_NameFlagAndInvalidRecipe(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "qwen", body["name"])

		writeJSON(w, http.StatusCreated, map[string]any{
			"id": "r1", "name": "qwen", "status": "invalid", "version": 1,
			"provenance": map[string]any{"diagnostics": []map[string]any{
				{"severity": "error", "code": "image", "message": "not the installation's runtime image"},
			}},
		})
	}))

	out, err := run(t, "create", recipeFile(t, barePayload), "--name", "qwen")
	require.ErrorContains(t, err, "is invalid")
	assert.Contains(t, out, "not the installation's runtime image")
}

func TestCreate_FileTwiceIsRefused(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request is sent when the file is given twice")
	}))

	p := recipeFile(t, barePayload)

	_, err := run(t, "create", p, "--from-file", p, "--name", "x")
	require.ErrorContains(t, err, "not both")
}

func TestPatch_PayloadNeedsIfVersion(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("no request is sent without --if-version")
	}))

	_, err := run(t, "patch", "r1", recipeFile(t, barePayload))
	require.ErrorContains(t, err, "--if-version")
}

func TestPatch_ConflictSuggestsReread(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]any{"detail": "ifVersion 1 is not the current version 2"})
	}))

	_, err := run(t, "patch", "r1", recipeFile(t, barePayload), "--if-version", "1")
	require.ErrorContains(t, err, "dr recipe get r1")
}

func TestPatch_ForbiddenSuggestsCopy(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"detail": "The recipe belongs to another user"})
	}))

	_, err := run(t, "patch", "r1", "--name", "mine")
	require.ErrorContains(t, err, "dr recipe copy r1")
}

func TestCopy_ReadsThenCreates(t *testing.T) {
	var created map[string]any

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			assert.Equal(t, "2", r.URL.Query().Get("version"))
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "src", "name": "orig", "description": "from catalog",
				"payload":    map[string]any{"format": "vllm.recipe/v1", "recipe": map[string]any{"hf_id": "Qwen/Qwen3-32B"}},
				"descriptor": map[string]any{"engine": "vllm"},
			})
		case http.MethodPost:
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&created))
			writeJSON(w, http.StatusCreated, map[string]any{"id": "new", "name": "mine", "status": "available"})
		}
	}))

	_, err := run(t, "copy", "src", "--name", "mine", "--version", "2")
	require.NoError(t, err)
	assert.Equal(t, "mine", created["name"])
	assert.Equal(t, "from catalog", created["description"])
	assert.NotNil(t, created["payload"])
	assert.NotNil(t, created["descriptor"])
}

func TestDelete_YesAndNotFound(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "Recipe not found"})
	}))

	out, err := run(t, "delete", "gone", "--yes")
	require.NoError(t, err)
	assert.Contains(t, out, "No recipe found with id: gone")
}

func TestGet_JSONIsPure(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": "r1", "name": "q", "status": "available"})
	}))

	out, err := run(t, "get", "r1", "--output-format", "json")
	require.NoError(t, err)

	var env map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &env))
	assert.Contains(t, env, "recipe")
}
