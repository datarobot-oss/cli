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

func intPtr(v int) *int {
	return &v
}

func sampleListResponse() pipeline.DataPage[pipelines.ListItem] {
	return pipeline.DataPage[pipelines.ListItem]{
		Data: []pipelines.ListItem{
			{
				PipelineID:    "683c2a1b4f8e1a2b3c4d5e6f",
				Name:          "confluence_to_vdb",
				Mode:          "draft",
				IsActive:      true,
				LatestVersion: intPtr(3),
				CreatedAt:     time.Date(2026, 4, 28, 11, 42, 28, 0, time.UTC),
				UpdatedAt:     time.Date(2026, 4, 28, 12, 25, 11, 0, time.UTC),
			},
		},
		TotalCount: 1,
		Count:      1,
	}
}

func TestPrintListJSON(t *testing.T) {
	list := sampleListResponse()

	output := captureStdout(t, func() {
		err := pipeline.RenderPipelines(pipelines.OutputFormatJSON, list)
		require.NoError(t, err)
	})

	var parsed []interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	require.Len(t, parsed, 1)

	item := parsed[0].(map[string]interface{})
	assert.Equal(t, "confluence_to_vdb", item["name"])
	assert.Equal(t, "draft", item["mode"])
}

func TestPrintListHuman_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderPipelines(pipelines.OutputFormatText, pipeline.DataPage[pipelines.ListItem]{}))
	})

	assert.Contains(t, output, "No pipelines found.")
}

func TestPrintListHuman_RendersHeaderAndRow(t *testing.T) {
	list := sampleListResponse()

	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderPipelines(pipelines.OutputFormatText, list))
	})

	assert.Contains(t, output, "Showing 1 of 1")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "MODE")
	assert.Contains(t, output, "ACTIVE")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "UPDATED")
	assert.Contains(t, output, "683c2a1b4f8e1a2b3c4d5e6f")
	assert.Contains(t, output, "confluence_to_vdb")
	assert.Contains(t, output, "draft")
	assert.Contains(t, output, "true")
	assert.Contains(t, output, "v3")
	assert.Contains(t, output, "2026-04-28")
}

func TestPrintListHuman_NoLatestVersion(t *testing.T) {
	list := sampleListResponse()
	list.Data[0].LatestVersion = nil

	output := captureStdout(t, func() {
		require.NoError(t, pipeline.RenderPipelines(pipelines.OutputFormatText, list))
	})

	assert.Contains(t, output, "—")
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"--output-format", "yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_RejectsInvalidMode(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"--mode", "bogus"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

func TestCmd_HasExpectedFlags(t *testing.T) {
	cmd := Cmd()

	for _, name := range []string{"mode", "offset", "limit", "output-format"} {
		flag := cmd.Flags().Lookup(name)
		assert.NotNilf(t, flag, "expected --%s flag to be registered", name)
	}
}
