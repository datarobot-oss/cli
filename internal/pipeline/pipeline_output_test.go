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

package pipeline

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldValue returns the trailing value of the tabwriter line beginning with
// label (the whitespace padding between label and value is normalised away), or
// ("", false) when no such line exists.
func fieldValue(out, label string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, label) {
			return strings.TrimSpace(strings.TrimPrefix(line, label)), true
		}
	}

	return "", false
}

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

// ── RenderCreateResponse ────────────────────────────────────────────────────

func TestRenderCreateResponse_JSON(t *testing.T) {
	result := CreateResponse{
		PipelineID: "abc123",
		Name:       "my_pipeline",
		Version:    1,
		Status:     "READY",
		Mode:       "draft",
		TaskNames:  []string{"task_a", "task_b"},
		CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderCreateResponse(outputformat.OutputFormatJSON, result))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "abc123", parsed["id"])
	assert.Equal(t, "my_pipeline", parsed["name"])
	assert.EqualValues(t, 1, parsed["version"])
	assert.Equal(t, "READY", parsed["status"])
	assert.Equal(t, "draft", parsed["mode"])
}

func TestRenderCreateResponse_Human(t *testing.T) {
	result := CreateResponse{
		PipelineID: "abc123",
		Name:       "my_pipeline",
		Version:    2,
		Status:     "READY",
		Mode:       "draft",
		TaskNames:  []string{"task_a", "task_b"},
		CreatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderCreateResponse(outputformat.OutputFormatText, result))
	})

	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "my_pipeline")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "READY")
	assert.Contains(t, out, "draft")
	assert.Contains(t, out, "task_a, task_b")
}

func TestRenderCreateResponse_Human_NoTasks(t *testing.T) {
	result := CreateResponse{
		PipelineID: "abc123",
		Name:       "my_pipeline",
		Version:    1,
		Status:     "READY",
		Mode:       "draft",
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderCreateResponse(outputformat.OutputFormatText, result))
	})

	assert.Contains(t, out, emptyValuePlaceholder)
}

func TestRenderCreateResponse_Human_DraftPlaceholders(t *testing.T) {
	// A fresh draft (e.g. a clone, or POST /pipelines) returns no version or
	// status; the human output must show the placeholder for those two fields
	// specifically rather than a bare "0" / empty cell.
	result := CreateResponse{
		PipelineID: "p-2",
		Name:       "Clone of wf",
		Version:    0,
		Status:     "",
		Mode:       "draft",
		TaskNames:  []string{"e1"},
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderCreateResponse(outputformat.OutputFormatText, result))
	})

	got, ok := fieldValue(out, "Version:")
	require.True(t, ok, "Version field must be rendered")
	assert.Equal(t, emptyValuePlaceholder, got, "zero version must render as placeholder")

	got, ok = fieldValue(out, "Status:")
	require.True(t, ok, "Status field must be rendered")
	assert.Equal(t, emptyValuePlaceholder, got, "empty status must render as placeholder")

	// A populated field must NOT be turned into a placeholder.
	got, ok = fieldValue(out, "Tasks:")
	require.True(t, ok, "Tasks field must be rendered")
	assert.Equal(t, "e1", got)
}

func TestRenderCreateResponse_Human_LockedKeepsValues(t *testing.T) {
	// Guard the other direction: a locked pipeline with a real version/status
	// must render those values verbatim, not the draft placeholder.
	result := CreateResponse{
		PipelineID: "p-9",
		Name:       "locked_wf",
		Version:    3,
		Status:     "READY",
		Mode:       "locked",
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderCreateResponse(outputformat.OutputFormatText, result))
	})

	got, _ := fieldValue(out, "Version:")
	assert.Equal(t, "3", got)

	got, _ = fieldValue(out, "Status:")
	assert.Equal(t, "READY", got)
}

// ── RenderPipeline ──────────────────────────────────────────────────────────

func samplePipeline() Pipeline {
	return Pipeline{
		PipelineID:  "pid1",
		Name:        "confluence_to_vdb",
		Description: "test desc",
		Mode:        "draft",
		IsActive:    true,
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Versions: []PipelineVersion{
			{
				Version:   1,
				Status:    "READY",
				TaskNames: []string{"task_a"},
				CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
}

func TestRenderPipeline_JSON(t *testing.T) {
	p := samplePipeline()

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipeline(outputformat.OutputFormatJSON, p))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "pid1", parsed["id"])
	assert.Equal(t, "confluence_to_vdb", parsed["name"])
	assert.Equal(t, "draft", parsed["mode"])
}

func TestRenderPipeline_Human(t *testing.T) {
	p := samplePipeline()

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipeline(outputformat.OutputFormatText, p))
	})

	assert.Contains(t, out, "pid1")
	assert.Contains(t, out, "confluence_to_vdb")
	assert.Contains(t, out, "draft")
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "v1")
	assert.Contains(t, out, "READY")
	assert.Contains(t, out, "task_a")
}

func TestRenderPipeline_Human_ErrorDetail(t *testing.T) {
	p := samplePipeline()
	p.Versions[0].ErrorDetail = "compilation failed"

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipeline(outputformat.OutputFormatText, p))
	})

	assert.Contains(t, out, "compilation failed")
}

func TestRenderPipeline_Human_NoVersions(t *testing.T) {
	p := samplePipeline()
	p.Versions = nil

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipeline(outputformat.OutputFormatText, p))
	})

	assert.Contains(t, out, "pid1")
	assert.NotContains(t, out, "VERSION")
}

func TestRenderPipeline_Human_NoDescription(t *testing.T) {
	p := samplePipeline()
	p.Description = ""

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipeline(outputformat.OutputFormatText, p))
	})

	assert.Contains(t, out, emptyValuePlaceholder)
}

// ── RenderPipelines ─────────────────────────────────────────────────────────

func sampleListItem() ListItem {
	ver := 3

	return ListItem{
		PipelineID:    "pid1",
		Name:          "confluence_to_vdb",
		Mode:          "draft",
		IsActive:      true,
		LatestVersion: &ver,
		CreatedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestRenderPipelines_JSON(t *testing.T) {
	page := DataPage[ListItem]{
		Data:       []ListItem{sampleListItem()},
		TotalCount: 1,
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipelines(outputformat.OutputFormatJSON, page))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	items, ok := parsed["pipelines"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 1)
}

func TestRenderPipelines_Human(t *testing.T) {
	page := DataPage[ListItem]{
		Data:       []ListItem{sampleListItem()},
		TotalCount: 1,
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipelines(outputformat.OutputFormatText, page))
	})

	assert.Contains(t, out, "pid1")
	assert.Contains(t, out, "confluence_to_vdb")
	assert.Contains(t, out, "draft")
	assert.Contains(t, out, "v3")
}

func TestRenderPipelines_Human_Empty(t *testing.T) {
	page := DataPage[ListItem]{}

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipelines(outputformat.OutputFormatText, page))
	})

	assert.Contains(t, out, "No pipelines found.")
}

func TestRenderPipelines_Human_NoLatestVersion(t *testing.T) {
	item := sampleListItem()
	item.LatestVersion = nil

	page := DataPage[ListItem]{Data: []ListItem{item}, TotalCount: 1}

	out := captureStdout(t, func() {
		require.NoError(t, RenderPipelines(outputformat.OutputFormatText, page))
	})

	assert.Contains(t, out, emptyValuePlaceholder)
}
