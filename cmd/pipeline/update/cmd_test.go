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

package update

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

func sampleUpdateResponse() pipeline.CreateResponse {
	return pipeline.CreateResponse{
		PipelineID: "683c2a1b4f8e1a2b3c4d5e6f",
		Name:       "confluence_to_vdb",
		Version:    2,
		Status:     "READY",
		Mode:       "draft",
		TaskNames:  []string{"create_vector_database"},
		CreatedAt:  time.Date(2026, 4, 28, 12, 24, 54, 0, time.UTC),
	}
}

func TestPrintUpdateJSON(t *testing.T) {
	resp := sampleUpdateResponse()

	output := captureStdout(t, func() {
		err := pipeline.RenderCreateResponse(pipeline.OutputFormatJSON, resp)
		require.NoError(t, err)
	})

	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Equal(t, resp.PipelineID, parsed["id"])
	assert.EqualValues(t, 2, parsed["version"])
	assert.Equal(t, "READY", parsed["status"])
}

func TestPrintUpdateHuman_WithTasks(t *testing.T) {
	resp := sampleUpdateResponse()

	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(pipeline.OutputFormatText, resp))
	})

	assert.Contains(t, output, resp.PipelineID)
	assert.Contains(t, output, "confluence_to_vdb")
	assert.Contains(t, output, "2")
	assert.Contains(t, output, "READY")
	assert.Contains(t, output, "draft")
	assert.Contains(t, output, "create_vector_database")
}

func TestPrintUpdateHuman_NoTasks(t *testing.T) {
	resp := sampleUpdateResponse()
	resp.TaskNames = nil

	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(pipeline.OutputFormatText, resp))
	})

	assert.Contains(t, output, "—")
}

func TestCmd_RequiresPipelineID(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_RequiresFilePath(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-id"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a file path is required")
}

func TestCmd_RejectsBothPositionalAndFromFile(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-id", "a.py", "--from-file=b.py"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-id", "some-file.py", "--output-format", "yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"output-format", "from-file"} {
		flag := cmd.Flags().Lookup(name)
		assert.NotNilf(t, flag, "expected --%s flag to be registered", name)
	}
}

// TestCmd_FromFileEqualsSyntax ensures the documented --from-file=<path>
// form parses correctly (cobra accepts both `--from-file value` and
// `--from-file=value`; we exercise the equals form here).
func TestCmd_FromFileEqualsSyntax(t *testing.T) {
	cmd := Cmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.ParseFlags([]string{"--from-file=./my_pipeline.py"})
	require.NoError(t, err)

	flag := cmd.Flags().Lookup("from-file")
	require.NotNil(t, flag)
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
