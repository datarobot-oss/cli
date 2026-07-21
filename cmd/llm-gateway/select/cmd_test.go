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

package selectcmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLLMs = []drapi.LLM{
	{LlmID: "llm-001", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true},
	{LlmID: "llm-002", Name: "Claude 3.5", Provider: "anthropic", Model: "claude-3-5-sonnet", IsActive: true},
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

// newTestCmd returns Cmd() with PreRunE stripped, wired to a writable output buffer.
func newTestCmd(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	var buf bytes.Buffer

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	return cmd, &buf
}

// --- findByID ---

func TestFindByID_Found(t *testing.T) {
	id, err := findByID(testLLMs, "llm-002")

	require.NoError(t, err)
	assert.Equal(t, "llm-002", id)
}

func TestFindByID_NotFound(t *testing.T) {
	_, err := findByID(testLLMs, "does-not-exist")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestFindByID_Empty(t *testing.T) {
	_, err := findByID(nil, "llm-001")

	require.Error(t, err)
}

func TestFindByID_DeployedID(t *testing.T) {
	llms := []drapi.LLM{
		{LlmID: "llm-001", Name: "GPT-4o", Provider: "azure", Model: "gpt-4o", IsActive: true, Kind: drapi.LLMKindGateway},
		{LlmID: "6650f0aa11bb22cc33dd44ee", Name: "Support RAG LLM", Model: "datarobot/datarobot-deployed-llm", IsActive: true, Kind: drapi.LLMKindDeployed, DeploymentID: "6650f0aa11bb22cc33dd44ee"},
	}

	id, err := findByID(llms, "6650f0aa11bb22cc33dd44ee")

	require.NoError(t, err)
	assert.Equal(t, "6650f0aa11bb22cc33dd44ee", id)
}

// --- select with direct arg ---

func TestSelectCmd_DirectArg_Valid(t *testing.T) {
	setupLLMServer(t, testLLMs)
	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", t.TempDir())

	cmd, buf := newTestCmd(t)
	cmd.SetArgs([]string{"llm-001"})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, "llm-001", viperx.GetString(config.DefaultLLMID))
	assert.Contains(t, buf.String(), "llm-001")
}

func TestSelectCmd_DirectArg_Invalid(t *testing.T) {
	setupLLMServer(t, testLLMs)

	cmd, _ := newTestCmd(t)
	cmd.SetArgs([]string{"not-a-real-llm"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-real-llm")
}

// TestSelectCmd_InvalidID_PartialFailure routes the two sources independently:
// the catalog answers, the deployments endpoint 500s. The deployed-source
// failure is explicit (not an incidental decode error), so an unknown id errors
// with both the id and the unavailable-source note.
func TestSelectCmd_InvalidID_PartialFailure(t *testing.T) {
	body, err := json.Marshal(drapi.LLMList{LLMs: testLLMs, Count: len(testLLMs), TotalCount: len(testLLMs)})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deployments") {
			http.Error(w, "boom", http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	cmd, _ := newTestCmd(t)
	cmd.SetArgs([]string{"not-a-real-llm"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-real-llm")
	assert.Contains(t, err.Error(), "DataRobot-deployed LLMs unavailable")
}

func TestSelectCmd_DirectArg_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	cmd, _ := newTestCmd(t)
	cmd.SetArgs([]string{"llm-001"})

	err := cmd.Execute()
	assert.Error(t, err)
}

// --- runPicker edge case (no TUI) ---

func TestRunPicker_EmptyCatalog(t *testing.T) {
	_, err := runPicker(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active LLMs")
}
