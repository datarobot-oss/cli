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

package lock

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

func sample() pipeline.CreateResponse {
	return pipeline.CreateResponse{
		PipelineID: "abc",
		Name:       "promo",
		Version:    3,
		Status:     "READY",
		Mode:       "locked",
		TaskNames:  []string{"e1", "e2"},
		CreatedAt:  time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
	}
}

func TestPrintLockJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(pipelines.OutputFormatJSON, sample()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "abc", parsed["id"])
	assert.Equal(t, "locked", parsed["mode"])
	assert.EqualValues(t, 3, parsed["version"])
}

func TestPrintLockHuman(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(pipelines.OutputFormatText, sample()))
	})

	assert.Contains(t, output, "abc")
	assert.Contains(t, output, "locked")
	assert.Contains(t, output, "3")
	assert.Contains(t, output, "e1, e2")
}

func TestPrintLockHuman_NoTasks(t *testing.T) {
	resp := sample()
	resp.TaskNames = nil

	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(pipelines.OutputFormatText, resp))
	})

	assert.Contains(t, output, "—")
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"abc", "--output-format", "yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}
