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

package list

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLLMs = []drapi.LLM{
	{LlmID: "llm-001", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true, Description: "flagship multimodal model", ContextSize: 128000},
	{LlmID: "llm-002", Name: "Claude 3.5", Provider: "anthropic", Model: "claude-3-5-sonnet", IsActive: true, Description: "balanced reasoning model", ContextSize: 200000},
}

// testMixedLLMs pairs a gateway model with a deployed LLM. The deployed row
// carries no provider/context and the litellm sentinel model; its LlmID is the
// deployment id.
var testMixedLLMs = []drapi.LLM{
	{LlmID: "llm-001", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true, ContextSize: 128000, Kind: drapi.LLMKindGateway},
	{LlmID: "6650f0aa11bb22cc33dd44ee", Name: "Support RAG LLM", Model: "datarobot/datarobot-deployed-llm", IsActive: true, Kind: drapi.LLMKindDeployed, DeploymentID: "6650f0aa11bb22cc33dd44ee"},
}

// setupLLMServer starts an httptest.Server serving a fixed LLM catalog and wires viperx config.
func setupLLMServer(t *testing.T, llms []drapi.LLM) {
	t.Helper()

	body := drapi.LLMList{LLMs: llms, Count: len(llms), TotalCount: len(llms)}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))

	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer

	_, _ = io.Copy(&buf, r)

	return buf.String()
}

// newTestCmd builds a minimal root → list command tree with PreRunE stripped.
func newTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "dr"}

	var rootOutputFormat outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	listCmd := Cmd()
	listCmd.PreRunE = nil
	root.AddCommand(listCmd)

	return root
}

// --- toLLMOutputs ---

func TestToLLMOutputs_Basic(t *testing.T) {
	outputs := toLLMOutputs(testLLMs, "")

	require.Len(t, outputs, 2)
	assert.Equal(t, "llm-001", outputs[0].ID)
	assert.Equal(t, "GPT-4o", outputs[0].Name)
	assert.Equal(t, "azure", outputs[0].Provider)
	assert.Equal(t, "gpt-4o", outputs[0].Model)
	assert.Equal(t, "flagship multimodal model", outputs[0].Description)
	assert.Equal(t, 128000, outputs[0].ContextSize)
	assert.False(t, outputs[0].Selected)
	assert.False(t, outputs[1].Selected)
}

func TestToLLMOutputs_SelectedMarked(t *testing.T) {
	outputs := toLLMOutputs(testLLMs, "llm-002")

	assert.False(t, outputs[0].Selected)
	assert.True(t, outputs[1].Selected)
}

func TestToLLMOutputs_Empty(t *testing.T) {
	assert.Empty(t, toLLMOutputs(nil, ""))
	assert.Empty(t, toLLMOutputs([]drapi.LLM{}, "any"))
}

// --- printLLMTable ---

func TestPrintLLMTable_SelectedPrefix(t *testing.T) {
	out := captureStdout(t, func() {
		printLLMTable(testLLMs, "llm-001")
	})

	assert.Contains(t, out, "* llm-001")
	assert.Contains(t, out, "  llm-002")
}

func TestPrintLLMTable_NoneSelected(t *testing.T) {
	out := captureStdout(t, func() {
		printLLMTable(testLLMs, "")
	})

	assert.NotContains(t, out, "* ")
	assert.Contains(t, out, "  llm-001")
	assert.Contains(t, out, "  llm-002")
}

// The table shows a CONTEXT column but deliberately omits description
// (it wraps into unreadable multi-line rows across a large catalog).
func TestPrintLLMTable_ContextColumnNoDescription(t *testing.T) {
	out := captureStdout(t, func() {
		printLLMTable(testLLMs, "")
	})

	assert.Contains(t, out, "CONTEXT")
	assert.Contains(t, out, "128000")
	assert.Contains(t, out, "200000")
	assert.NotContains(t, out, "flagship multimodal model")
	assert.NotContains(t, out, "balanced reasoning model")
}

func TestFormatContextSize(t *testing.T) {
	assert.Equal(t, "128000", formatContextSize(128000))
	assert.Equal(t, "-", formatContextSize(0))
	assert.Equal(t, "-", formatContextSize(-1))
}

// --- full command ---

func TestListCmd_TableOutput(t *testing.T) {
	setupLLMServer(t, testLLMs)

	root := newTestCmd(t)
	root.SetArgs([]string{"list"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	assert.Contains(t, out, "llm-001")
	assert.Contains(t, out, "llm-002")
}

func TestListCmd_TableOutput_SelectedMarker(t *testing.T) {
	setupLLMServer(t, testLLMs)
	viperx.Set(config.DefaultLLMID, "llm-001")

	root := newTestCmd(t)
	root.SetArgs([]string{"list"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	assert.Contains(t, out, "* llm-001")
	assert.Contains(t, out, "  llm-002")
}

func TestListCmd_JSONOutput(t *testing.T) {
	setupLLMServer(t, testLLMs)

	root := newTestCmd(t)
	root.SetArgs([]string{"list", "--output-format", "json"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	var envelope struct {
		LLMs []LLMOutput `json:"llms"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Len(t, envelope.LLMs, 2)
	assert.Equal(t, "llm-001", envelope.LLMs[0].ID)
	assert.Equal(t, "llm-002", envelope.LLMs[1].ID)
	assert.Equal(t, "flagship multimodal model", envelope.LLMs[0].Description)
	assert.Equal(t, 128000, envelope.LLMs[0].ContextSize)
	assert.False(t, envelope.LLMs[0].Selected)
	assert.False(t, envelope.LLMs[1].Selected)

	// Lock the wire key as snake_case: the contract CFX-6981 consumes.
	assert.Contains(t, out, `"context_size"`)
}

func TestListCmd_JSONOutput_SelectedField(t *testing.T) {
	setupLLMServer(t, testLLMs)
	viperx.Set(config.DefaultLLMID, "llm-002")

	root := newTestCmd(t)
	root.SetArgs([]string{"list", "--output-format", "json"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	var envelope struct {
		LLMs []LLMOutput `json:"llms"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	assert.False(t, envelope.LLMs[0].Selected)
	assert.True(t, envelope.LLMs[1].Selected)
}

func TestListCmd_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	root := newTestCmd(t)
	root.SetArgs([]string{"list"})

	err := root.Execute()
	assert.Error(t, err)
}

// --- deployed-LLM union ---

func TestToLLMOutputs_DeployedFields(t *testing.T) {
	outputs := toLLMOutputs(testMixedLLMs, "")

	require.Len(t, outputs, 2)

	assert.Equal(t, "gateway", outputs[0].Source)
	assert.Empty(t, outputs[0].DeploymentID)

	assert.Equal(t, "deployed", outputs[1].Source)
	assert.Equal(t, "6650f0aa11bb22cc33dd44ee", outputs[1].ID)
	assert.Equal(t, "6650f0aa11bb22cc33dd44ee", outputs[1].DeploymentID)
	assert.Equal(t, "datarobot/datarobot-deployed-llm", outputs[1].Model)
}

// TestToLLMOutputs_DeployedJSONKeys locks the wire contract CFX-6980 consumes:
// snake_case source + deployment_id present on every entry.
func TestToLLMOutputs_DeployedJSONKeys(t *testing.T) {
	data, err := json.Marshal(toLLMOutputs(testMixedLLMs, ""))
	require.NoError(t, err)

	out := string(data)
	assert.Contains(t, out, `"source":"gateway"`)
	assert.Contains(t, out, `"source":"deployed"`)
	assert.Contains(t, out, `"deployment_id":"6650f0aa11bb22cc33dd44ee"`)
}

func TestPrintLLMTable_DeployedRow(t *testing.T) {
	out := captureStdout(t, func() {
		printLLMTable(testMixedLLMs, "")
	})

	// SOURCE column carries the kind, and the deployed row shows its label + id.
	assert.Contains(t, out, "deployed")
	assert.Contains(t, out, "Support RAG LLM")
	assert.Contains(t, out, "6650f0aa11bb22cc33dd44ee")

	// The sentinel model is blanked to "-" in the table (JSON-only contract).
	assert.NotContains(t, out, "datarobot/datarobot-deployed-llm")
}
