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

package get

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
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

func samplePipeline() pipeline.Pipeline {
	return pipeline.Pipeline{
		PipelineID:  "683c2a1b4f8e1a2b3c4d5e6f",
		Name:        "confluence_to_vdb",
		Description: "test",
		Mode:        "draft",
		IsActive:    true,
		CreatedAt:   pipeline.Time{Time: time.Date(2026, 4, 28, 11, 42, 28, 0, time.UTC)},
		UpdatedAt:   pipeline.Time{Time: time.Date(2026, 4, 28, 12, 25, 11, 0, time.UTC)},
		Versions: []pipeline.PipelineVersion{
			{
				Version:       1,
				Status:        "READY",
				PipelineName:  "confluence_to_vdb",
				TaskNames:     []string{"create_vector_database", "ingest_confluence_files"},
				PythonVersion: "3.12",
				CreatedAt:     pipeline.Time{Time: time.Date(2026, 4, 28, 11, 42, 28, 0, time.UTC)},
			},
			{
				Version:       2,
				Status:        "FAILED",
				PipelineName:  "confluence_to_vdb",
				PythonVersion: "3.12",
				ErrorDetail:   "boom",
				CreatedAt:     pipeline.Time{Time: time.Date(2026, 4, 28, 12, 25, 11, 0, time.UTC)},
			},
		},
	}
}

func TestPrintGetJSON(t *testing.T) {
	pipeline := samplePipeline()

	output := captureStdout(t, func() {
		err := printGetJSON(pipeline)
		require.NoError(t, err)
	})

	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
	assert.Equal(t, pipeline.PipelineID, parsed["pipeline_id"])
	assert.Equal(t, "confluence_to_vdb", parsed["name"])

	versions, ok := parsed["versions"].([]interface{})
	require.True(t, ok)
	require.Len(t, versions, 2)
}

func TestPrintGetHuman_RendersHeaderAndVersions(t *testing.T) {
	pipeline := samplePipeline()

	output := captureStdout(t, func() {
		printGetHuman(pipeline)
	})

	assert.Contains(t, output, "ID:          "+pipeline.PipelineID)
	assert.Contains(t, output, "Name:        confluence_to_vdb")
	assert.Contains(t, output, "Description: test")
	assert.Contains(t, output, "Mode:        draft")
	assert.Contains(t, output, "Active:      true")
	assert.Contains(t, output, "Versions (2):")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "PYTHON")
	assert.Contains(t, output, "CREATED")
	assert.Contains(t, output, "TASKS")
	assert.Contains(t, output, "v1")
	assert.Contains(t, output, "v2")
	assert.Contains(t, output, "create_vector_database, ingest_confluence_files")
	assert.Contains(t, output, "v2 error: boom")
}

func TestPrintGetHuman_BlankDescriptionFallsBack(t *testing.T) {
	pipeline := samplePipeline()
	pipeline.Description = ""

	output := captureStdout(t, func() {
		printGetHuman(pipeline)
	})

	assert.Contains(t, output, "Description: \u2014")
}

func TestPrintGetHuman_NoVersions(t *testing.T) {
	pipeline := samplePipeline()
	pipeline.Versions = nil

	output := captureStdout(t, func() {
		printGetHuman(pipeline)
	})

	assert.NotContains(t, output, "Versions (")
}

func TestCmd_RequiresArg(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_RejectsInvalidOutput(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"some-id", "--output", "yaml"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_HasOutputFlag(t *testing.T) {
	cmd := Cmd()
	assert.NotNil(t, cmd.Flags().Lookup("output"))
}

func TestHandleGetError_NotFoundPrintsFriendlyMessage(t *testing.T) {
	httpErr := &drapi.HTTPError{StatusCode: 404, URL: "http://example/api/v2/pipelines/abc"}

	output := captureStdout(t, func() {
		err := handleGetError(httpErr, "abc")
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "No pipeline found with id: abc")
}

func TestHandleGetError_OtherErrorsPassThrough(t *testing.T) {
	otherHTTP := &drapi.HTTPError{StatusCode: 500, URL: "http://example/api/v2/pipelines/abc"}

	output := captureStdout(t, func() {
		err := handleGetError(otherHTTP, "abc")
		require.Error(t, err)
		assert.Same(t, otherHTTP, err)
	})

	assert.NotContains(t, output, "No pipeline found")
}

func TestHandleGetError_NonHTTPErrorPassesThrough(t *testing.T) {
	plain := errors.New("network unreachable")

	err := handleGetError(plain, "abc")
	require.Error(t, err)
	assert.Equal(t, plain, err)
}
