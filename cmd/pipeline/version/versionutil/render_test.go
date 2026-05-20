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

package versionutil

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

func sampleVersion() pipeline.PipelineVersion {
	return pipeline.PipelineVersion{
		Version:       2,
		Status:        "READY",
		PipelineName:  "wf",
		TaskNames:     []string{"e1", "e2"},
		PythonVersion: "3.12",
		CreatedAt:     pipeline.Time{Time: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)},
	}
}

func TestPrintVersionJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintVersionJSON(sampleVersion()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.EqualValues(t, 2, parsed["version"])
	assert.Equal(t, "READY", parsed["status"])
	assert.Equal(t, "wf", parsed["lattice_name"])
}

func TestPrintVersionHuman_Full(t *testing.T) {
	output := captureStdout(t, func() { PrintVersionHuman(sampleVersion()) })
	assert.Contains(t, output, "Version:        v2")
	assert.Contains(t, output, "Pipeline:       wf")
	assert.Contains(t, output, "Status:         READY")
	assert.Contains(t, output, "Python Version: 3.12")
	assert.Contains(t, output, "Tasks:          e1, e2")
	assert.Contains(t, output, "Created:        2026-04-29T10:00:00Z")
}

func TestPrintVersionHuman_FillsDefaultsForMissing(t *testing.T) {
	v := sampleVersion()
	v.TaskNames = nil
	v.PythonVersion = ""

	output := captureStdout(t, func() { PrintVersionHuman(v) })
	assert.Contains(t, output, "Python Version: \u2014")
	assert.Contains(t, output, "Tasks:          \u2014")
}

func TestPrintVersionHuman_ShowsErrorDetail(t *testing.T) {
	v := sampleVersion()
	v.ErrorDetail = "syntax error"

	output := captureStdout(t, func() { PrintVersionHuman(v) })
	assert.Contains(t, output, "Error:          syntax error")
}

func TestPrintVersionListJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintVersionListJSON([]pipeline.PipelineVersion{sampleVersion()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 1)
	assert.EqualValues(t, 2, parsed[0]["version"])
}

func TestPrintVersionListHuman_Empty(t *testing.T) {
	output := captureStdout(t, func() { PrintVersionListHuman(nil) })
	assert.Contains(t, output, "No versions found")
}

func TestPrintVersionListHuman_RendersTable(t *testing.T) {
	output := captureStdout(t, func() {
		PrintVersionListHuman([]pipeline.PipelineVersion{sampleVersion()})
	})

	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "PYTHON")
	assert.Contains(t, output, "v2")
	assert.Contains(t, output, "READY")
	assert.Contains(t, output, "3.12")
	assert.Contains(t, output, "e1, e2")
}
