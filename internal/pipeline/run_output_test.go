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
	"encoding/json"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleRun() Run {
	ver := 2

	return Run{
		RunID:              "d-1",
		PipelineID:         "p-1",
		VersionID:          &ver,
		InputID:            "in-1",
		CovalentDispatchID: "cov-42",
		TriggeredBy:        "user@example.com",
		Status:             RunStatusPending,
		CreatedAt:          time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 4, 29, 10, 5, 0, 0, time.UTC),
	}
}

// ── toRunJSON remapping ──────────────────────────────────────────────────────

func TestToRunJSON_RemapsWireFields(t *testing.T) {
	j := toRunJSON(sampleRun())

	assert.Equal(t, "d-1", j.RunID)
	assert.Equal(t, "p-1", j.PipelineID)
	assert.Equal(t, "in-1", j.InputID)
	assert.Equal(t, "cov-42", j.CovalentRunID)
	assert.Equal(t, RunStatusPending, j.Status)
}

func TestToRunJSON_FormatsTimestampsAsRFC3339(t *testing.T) {
	j := toRunJSON(sampleRun())

	assert.Equal(t, "2026-04-29T10:00:00Z", j.CreatedAt)
	assert.Equal(t, "2026-04-29T10:05:00Z", j.UpdatedAt)
}

func TestToRunJSON_JSONKeysUseCliVocabulary(t *testing.T) {
	data, err := json.Marshal(toRunJSON(sampleRun()))
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "run_id", "wire 'id' must be remapped to 'run_id'")
	assert.Contains(t, raw, "covalent_run_id", "wire 'covalentDispatchId' must be remapped to 'covalent_run_id'")
	assert.NotContains(t, raw, "id", "raw wire key 'id' must not appear in CLI output")
	assert.NotContains(t, raw, "covalentDispatchId", "raw wire key must not appear in CLI output")
}

// ── RenderRun ────────────────────────────────────────────────────────────────

func TestRenderRun_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderRun(outputformat.OutputFormatJSON, sampleRun()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "d-1", parsed["run_id"])
	assert.Equal(t, "cov-42", parsed["covalent_run_id"])
	assert.Equal(t, "2026-04-29T10:00:00Z", parsed["created_at"])
}

func TestRenderRun_Human(t *testing.T) {
	out := captureStdout(t, func() { PrintRunHuman(sampleRun()) })

	assert.Contains(t, out, "d-1")
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, "in-1")
	assert.Contains(t, out, RunStatusPending)
}

// ── RenderRunStatus ──────────────────────────────────────────────────────────

func TestRenderRunStatus_JSON(t *testing.T) {
	s := RunStatus{RunID: "d-1", Status: RunStatusRunning, CovalentDispatchID: "cov-42"}

	out := captureStdout(t, func() {
		require.NoError(t, RenderRunStatus(outputformat.OutputFormatJSON, s))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "d-1", parsed["run_id"])
	assert.Equal(t, "cov-42", parsed["covalent_run_id"])
	assert.NotContains(t, parsed, "id")
	assert.NotContains(t, parsed, "covalentDispatchId")
}

// ── PrintRunListJSON ─────────────────────────────────────────────────────────

func TestPrintRunListJSON_RemapsFields(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, PrintRunListJSON([]Run{sampleRun()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "d-1", parsed[0]["run_id"])
	assert.Equal(t, "cov-42", parsed[0]["covalent_run_id"])
}

func TestPrintRunListHuman_Empty(t *testing.T) {
	out := captureStdout(t, func() { PrintRunListHuman(nil) })
	assert.Contains(t, out, "No runs found")
}

func TestPrintRunListHuman_RendersTable(t *testing.T) {
	out := captureStdout(t, func() { PrintRunListHuman([]Run{sampleRun()}) })

	assert.Contains(t, out, "RUN ID")
	assert.Contains(t, out, "d-1")
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, RunStatusPending)
}
