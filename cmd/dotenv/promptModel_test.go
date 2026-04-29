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

package dotenv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupLLMTestServer starts an httptest.Server that handles token verification
// and the LLM catalog endpoint, responding with catalogStatus and optional JSON body.
func setupLLMTestServer(t *testing.T, catalogStatus int, catalogBody any) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v2/version/":
			w.WriteHeader(http.StatusOK)

		case strings.HasPrefix(r.URL.Path, "/api/v2/genai/llmgw/catalog"):
			w.WriteHeader(catalogStatus)

			if catalogBody != nil {
				_ = json.NewEncoder(w).Encode(catalogBody)
			}
		}
	}))

	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

func TestLLMsToPromptOptions(t *testing.T) {
	llms := []drapi.LLM{
		{LlmID: "1", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true},
		{LlmID: "2", Name: "Claude 3", Provider: "anthropic", Model: "claude-3-sonnet", IsActive: true},
	}

	options := llmsToPromptOptions(llms)

	assert.Len(t, options, 2)

	assert.Equal(t, envbuilder.PromptOption{
		Blank:    false,
		Checked:  false,
		Name:     "GPT-4o (azure)",
		Value:    "datarobot/gpt-4o",
		Requires: "",
	}, options[0])

	assert.Equal(t, envbuilder.PromptOption{
		Blank:    false,
		Checked:  false,
		Name:     "Claude 3 (anthropic)",
		Value:    "datarobot/claude-3-sonnet",
		Requires: "",
	}, options[1])
}

func TestLLMsToPromptOptions_Empty(t *testing.T) {
	options := llmsToPromptOptions([]drapi.LLM{})

	assert.Empty(t, options)
}

// TestNewLLMListPrompt_Unauthorized verifies that when the API returns 401,
// the model is set to an error state with an authentication failure message.
func TestNewLLMListPrompt_Unauthorized(t *testing.T) {
	setupLLMTestServer(t, http.StatusUnauthorized, nil)

	prompt := envbuilder.UserPrompt{Type: "llmgw_catalog", Env: "LLM_VAR"}

	pm, cmd := newLLMListPrompt(prompt, nil)

	assert.Nil(t, cmd)
	assert.Contains(t, pm.prompt.Type.String(), "error")
	assert.Contains(t, pm.prompt.Help, "Authentication failed")
}

// TestNewLLMListPrompt_NotFound verifies that when the API returns 404,
// the model is set to an error state with a resource not found message.
func TestNewLLMListPrompt_NotFound(t *testing.T) {
	setupLLMTestServer(t, http.StatusNotFound, nil)

	prompt := envbuilder.UserPrompt{Type: "llmgw_catalog", Env: "LLM_VAR"}

	pm, cmd := newLLMListPrompt(prompt, nil)

	assert.Nil(t, cmd)
	assert.Contains(t, pm.prompt.Type.String(), "error")
	assert.Contains(t, pm.prompt.Help, "Requested resource not found")
}

// TestNewLLMListPrompt_Timeout verifies that when the API returns 408,
// the model is set to an error state with a timeout message.
// Note: 408 Request Timeout triggers the "Timeout" string check in newLLMListPrompt
// without requiring a 30-second real client timeout.
func TestNewLLMListPrompt_Timeout(t *testing.T) {
	setupLLMTestServer(t, http.StatusRequestTimeout, nil)

	prompt := envbuilder.UserPrompt{Type: "llmgw_catalog", Env: "LLM_VAR"}

	pm, cmd := newLLMListPrompt(prompt, nil)

	assert.Nil(t, cmd)
	assert.Contains(t, pm.prompt.Type.String(), "error")
	assert.Contains(t, pm.prompt.Help, "Request timed out")
}

// TestNewLLMListPrompt_EmptyLLMs verifies that when the API returns a 200 response
// with an empty LLM list, the model is set to an error state indicating no LLMs are available.
func TestNewLLMListPrompt_EmptyLLMs(t *testing.T) {
	setupLLMTestServer(t, http.StatusOK, drapi.LLMList{
		LLMs: []drapi.LLM{}, Count: 0, TotalCount: 0,
	})

	prompt := envbuilder.UserPrompt{Type: "llmgw_catalog", Env: "LLM_VAR"}

	pm, cmd := newLLMListPrompt(prompt, nil)

	assert.Nil(t, cmd)
	assert.Contains(t, pm.prompt.Type.String(), "error")
	assert.Contains(t, pm.prompt.Help, "No available LLMs")
}

// TestNewLLMListPrompt_Success verifies that when the API returns a valid LLM list,
// the model is a list prompt with all returned LLMs as selectable options.
func TestNewLLMListPrompt_Success(t *testing.T) {
	setupLLMTestServer(t, http.StatusOK, drapi.LLMList{
		LLMs: []drapi.LLM{
			{LlmID: "1", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true},
			{LlmID: "2", Name: "Claude 3", Provider: "anthropic", Model: "claude-3-sonnet", IsActive: true},
		},
		Count: 2, TotalCount: 2,
	})

	prompt := envbuilder.UserPrompt{Type: "llmgw_catalog", Env: "LLM_VAR"}

	pm, _ := newLLMListPrompt(prompt, nil)

	require.NotContains(t, pm.prompt.Type.String(), "error")
	assert.Len(t, pm.list.Items(), 2)
}

// TestPromptModelView_ErrorType verifies that when the prompt type contains "error",
// View renders the variable name, error help message, and back navigation hint.
func TestPromptModelView_ErrorType(t *testing.T) {
	pm := promptModel{
		prompt: envbuilder.UserPrompt{
			Type: "error",
			Env:  "MY_VAR",
			Help: "Unable to retrieve LLMs.",
		},
	}

	view := pm.View()

	assert.Contains(t, view, "MY_VAR")
	assert.Contains(t, view, "Unable to retrieve LLMs.")
	assert.Contains(t, view, "ctrl-p back to previous")
}

// TestPromptModelView_Success verifies that when the prompt type is a non-error type,
// View renders the variable name, help text, text input, and back navigation hint.
func TestPromptModelView_Success(t *testing.T) {
	pm, _ := newTextInputPrompt(envbuilder.UserPrompt{
		Type: envbuilder.PromptTypeString,
		Env:  "MY_VAR",
		Help: "Enter a value",
	}, nil)

	view := pm.View()

	assert.Contains(t, view, "MY_VAR")
	assert.Contains(t, view, "Enter a value")
	assert.Contains(t, view, "ctrl-p back to previous")
}

// TestPromptModelView_WithDefault verifies that when prompt.Default is set,
// View renders the default value in the output.
func TestPromptModelView_WithDefault(t *testing.T) {
	pm, _ := newTextInputPrompt(envbuilder.UserPrompt{
		Type:    envbuilder.PromptTypeString,
		Env:     "MY_VAR",
		Default: "my-default",
	}, nil)

	view := pm.View()

	assert.Contains(t, view, "Default: my-default")
}

// TestPromptModelView_WithOptions verifies that when prompt.Options is non-empty,
// View renders the variable name and the selectable option names.
func TestPromptModelView_WithOptions(t *testing.T) {
	pm, _ := newListPrompt(envbuilder.UserPrompt{
		Type: envbuilder.PromptTypeString,
		Env:  "MY_VAR",
		Options: []envbuilder.PromptOption{
			{Name: "Option A", Value: "a"},
			{Name: "Option B", Value: "b"},
		},
	}, nil)

	view := pm.View()

	assert.Contains(t, view, "MY_VAR")
	assert.Contains(t, view, "Option A")
}

// TestPromptModelView_MultipleChoice verifies that when prompt.Multiple is true,
// View renders the multi-select keyboard hint alongside the options list.
func TestPromptModelView_MultipleChoice(t *testing.T) {
	pm, _ := newListPrompt(envbuilder.UserPrompt{
		Type:     envbuilder.PromptTypeString,
		Env:      "MY_VAR",
		Multiple: true,
		Options: []envbuilder.PromptOption{
			{Name: "Option A", Value: "a"},
			{Name: "Option B", Value: "b"},
		},
	}, nil)

	view := pm.View()

	assert.Contains(t, view, "space to toggle")
}
