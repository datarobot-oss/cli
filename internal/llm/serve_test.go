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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultServedName(t *testing.T) {
	assert.Equal(t, "kimi-k3.1-pro", DefaultServedName("moonshotai/Kimi-K3.1-Pro"))
	assert.Equal(t, "qwen3-0.6b", DefaultServedName("Qwen/Qwen3-0.6B"))
	assert.Equal(t, "a-b", DefaultServedName("org/__A b__"))
}

func TestBuildDeployRequest_InlineArtifact(t *testing.T) {
	body, err := BuildDeployRequest(DeployOptions{
		Name: "kimi", RecipeID: "r1", RecipeVersion: 2, Bundles: []string{"gpu.b200.x8"},
		Model: "moonshotai/Kimi-K3.1-Pro", ToolCalling: "off", TokenGrid: true, TokensPerSecond: 800,
	})
	require.NoError(t, err)

	raw, _ := json.Marshal(body)

	var got struct {
		Name     string `json:"name"`
		Artifact struct {
			Type string `json:"type"`
			Spec struct {
				Model struct {
					Source      map[string]string `json:"source"`
					ServedName  string            `json:"servedName"`
					ToolCalling *struct {
						Enabled bool `json:"enabled"`
					} `json:"toolCalling"`
					Reasoning any `json:"reasoning"`
				} `json:"model"`
			} `json:"spec"`
		} `json:"artifact"`
		Runtime struct {
			Recipe    map[string]any            `json:"recipe"`
			Workers   map[string][]string       `json:"workers"`
			TokenGrid map[string]any            `json:"tokenGrid"`
			Other     map[string]map[string]any `json:"-"`
		} `json:"runtime"`
	}

	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "llm", got.Artifact.Type)
	assert.Equal(t, map[string]string{"kind": "huggingFace", "repoId": "moonshotai/Kimi-K3.1-Pro"}, got.Artifact.Spec.Model.Source)
	assert.Equal(t, "kimi-k3.1-pro", got.Artifact.Spec.Model.ServedName)
	require.NotNil(t, got.Artifact.Spec.Model.ToolCalling)
	assert.False(t, got.Artifact.Spec.Model.ToolCalling.Enabled)
	assert.Nil(t, got.Artifact.Spec.Model.Reasoning, "auto omits the capability")
	assert.Equal(t, "r1", got.Runtime.Recipe["id"])
	assert.InDelta(t, 2, got.Runtime.Recipe["version"], 0)
	assert.Equal(t, []string{"gpu.b200.x8"}, got.Runtime.Workers["resourceBundles"])
	assert.InDelta(t, 800, got.Runtime.TokenGrid["tokensPerSecond"], 0)
}

func TestBuildDeployRequest_ReuseArtifactAndErrors(t *testing.T) {
	body, err := BuildDeployRequest(DeployOptions{Name: "n", RecipeID: "r", Bundles: []string{"b"}, ArtifactID: "a1"})
	require.NoError(t, err)
	assert.Equal(t, "a1", body["artifactId"])
	assert.NotContains(t, body, "artifact")

	_, err = BuildDeployRequest(DeployOptions{Name: "n", Bundles: []string{"b"}})
	require.ErrorContains(t, err, "no model")

	_, err = BuildDeployRequest(DeployOptions{Name: "n", Model: "m"})
	require.ErrorContains(t, err, "--bundle")

	_, err = BuildDeployRequest(DeployOptions{Name: "n", Bundles: []string{"b"}, Model: "m", ModelSource: "pvc"})
	require.ErrorContains(t, err, "--model-source")

	_, err = BuildDeployRequest(DeployOptions{Name: "n", Bundles: []string{"b"}, Model: "m", Reasoning: "maybe"})
	require.ErrorContains(t, err, "capability")
}

func TestRolloutRuntime_StripsResolvedAndInlineStorage(t *testing.T) {
	current := map[string]any{
		"resolvedRecipe": map[string]any{"id": "r1", "version": 3.0},
		"recipe":         map[string]any{"id": "r1"},
		"tokenGrid":      map[string]any{"enabled": true},
		"workers": map[string]any{
			"resourceBundles": []any{"gpu.b200.x8"},
			"resolvedBundle":  map[string]any{"id": "gpu.b200.x8"},
			"mounts": []any{map[string]any{
				"name": "modelWeight", "storageId": "s1",
				"storage": map[string]any{"purpose": "modelWeight"},
			}},
			"resolvedMounts": []any{},
		},
	}

	raw, err := RolloutRuntime(current, "r1", 4)
	require.NoError(t, err)

	var got map[string]any

	require.NoError(t, json.Unmarshal(raw, &got))
	assert.NotContains(t, got, "resolvedRecipe")
	assert.Equal(t, map[string]any{"id": "r1", "mode": "strict", "version": 4.0}, got["recipe"])
	assert.Equal(t, map[string]any{"enabled": true}, got["tokenGrid"])

	workers, _ := got["workers"].(map[string]any)
	assert.NotContains(t, workers, "resolvedBundle")
	assert.NotContains(t, workers, "resolvedMounts")
	assert.Equal(t, []any{map[string]any{"name": "modelWeight", "storageId": "s1"}}, workers["mounts"])

	_, stillThere := current["resolvedRecipe"]
	assert.True(t, stillThere, "the caller's runtime is not modified")
}

func TestChat_SendsModelHeaderAndBody(t *testing.T) {
	base := serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/endpoints/workloads/w1/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "qwen3", r.Header.Get(ModelHeader))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "qwen3", body["model"])
		assert.InDelta(t, 1, body["max_tokens"], 0)

		writeJSON(w, http.StatusOK, map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))

	res, err := Chat(context.Background(), ChatRequest{
		Endpoint: base + "/api/v2/endpoints/workloads/w1/", ServedName: "qwen3",
		Messages: SmokeMessages(), MaxTokens: 1,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "pong", res.Reply)
	assert.Equal(t, 4, res.Usage.TotalTokens)
}

func TestChat_Stream(t *testing.T) {
	base := serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		for _, part := range []string{"Hel", "lo"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
		}

		fmt.Fprint(w, "data: [DONE]\n\n")
	}))

	var live bytes.Buffer

	res, err := Chat(context.Background(), ChatRequest{
		Endpoint: base + "/api/v2/endpoints/workloads/w1/", ServedName: "m",
		Messages: SmokeMessages(), Stream: true,
	}, &live)
	require.NoError(t, err)
	assert.Equal(t, "Hello", res.Reply)
	assert.Equal(t, "Hello", live.String())
}

func TestChat_RefusesAnotherHost(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request is sent to another host")
	}))

	_, err := Chat(context.Background(), ChatRequest{
		Endpoint: "https://agent-router.enclave.example/", ServedName: "m", Messages: SmokeMessages(),
	}, nil)
	require.ErrorIs(t, err, ErrOffHost)
}

func TestServedName_RefusesNonLLMArtifact(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"type": "service", "spec": map[string]any{}})
	}))

	_, err := ServedName("a1")
	require.ErrorContains(t, err, "not an llm one")
}
