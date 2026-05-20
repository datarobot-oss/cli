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

package create

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()

	os.Stdout = old

	var buf bytes.Buffer

	_, _ = io.Copy(&buf, r)

	return buf.String()
}

func sampleCreateResponse() pipeline.CreateResponse {
	return pipeline.CreateResponse{
		PipelineID: "683c2a1b4f8e1a2b3c4d5e6f",
		Name:       "confluence_to_vdb",
		Version:    1,
		Status:     "READY",
		Mode:       "draft",
		TaskNames:  []string{"create_vector_database", "ingest_confluence_files"},
		CreatedAt:  pipeline.Time{Time: time.Date(2026, 4, 28, 11, 42, 28, 0, time.UTC)},
	}
}

func TestPrintCreateJSON(t *testing.T) {
	resp := sampleCreateResponse()

	output := captureStdout(t, func() {
		err := printCreateJSON(resp)
		require.NoError(t, err)
	})

	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Equal(t, resp.PipelineID, parsed["pipeline_id"])
	assert.Equal(t, resp.Name, parsed["name"])
	assert.Equal(t, "READY", parsed["status"])
	assert.Equal(t, "draft", parsed["mode"])
	assert.EqualValues(t, 1, parsed["version"])
}

func TestPrintCreateHuman_WithTasks(t *testing.T) {
	resp := sampleCreateResponse()

	output := captureStdout(t, func() {
		printCreateHuman(resp)
	})

	assert.Contains(t, output, resp.PipelineID)
	assert.Contains(t, output, "confluence_to_vdb")
	assert.Contains(t, output, "Version:      1")
	assert.Contains(t, output, "Status:       READY")
	assert.Contains(t, output, "Mode:         draft")
	assert.Contains(t, output, "create_vector_database, ingest_confluence_files")
}

func TestPrintCreateHuman_NoTasks(t *testing.T) {
	resp := sampleCreateResponse()
	resp.TaskNames = nil

	output := captureStdout(t, func() {
		printCreateHuman(resp)
	})

	assert.Contains(t, output, "Tasks:        \u2014")
}

func TestCmd_RequiresFilePath(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil // bypass auth

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a file path is required")
}

func TestCmd_RejectsBothPositionalAndFromFile(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"a.py", "--from-file=b.py"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

// TestCmd_FromFileEqualsSyntax ensures the documented --from-file=<path>
// form parses correctly (cobra accepts both `--from-file value` and
// `--from-file=value`; we exercise the equals form here).
func TestCmd_FromFileEqualsSyntax(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"--from-file=./my_pipeline.py"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	flag := cmd.Flags().Lookup("from-file")
	require.NotNil(t, flag)

	err := cmd.ParseFlags([]string{"--from-file=./my_pipeline.py"})
	require.NoError(t, err)
	assert.Equal(t, "./my_pipeline.py", flag.Value.String())
}

func TestResolveFilePath(t *testing.T) {
	t.Run("positional only", func(t *testing.T) {
		got, err := resolveFilePath([]string{"a.py"}, "")
		require.NoError(t, err)
		assert.Equal(t, "a.py", got)
	})

	t.Run("flag only", func(t *testing.T) {
		got, err := resolveFilePath(nil, "b.py")
		require.NoError(t, err)
		assert.Equal(t, "b.py", got)
	})

	t.Run("both supplied", func(t *testing.T) {
		_, err := resolveFilePath([]string{"a.py"}, "b.py")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not both")
	})

	t.Run("neither supplied", func(t *testing.T) {
		_, err := resolveFilePath(nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-file.py", "--output", "yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil // bypass auth

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_RejectsInvalidMode(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-file.py", "--mode", "bogus"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil // bypass auth

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"description", "mode", "output", "from-file"} {
		flag := cmd.Flags().Lookup(name)
		assert.NotNilf(t, flag, "expected --%s flag to be registered", name)
	}
}
