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

package runutil

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

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

func intPtr(v int) *int { return &v }

func sampleDraftRun() pipeline.Run {
	return pipeline.Run{
		RunID:       "d-1",
		PipelineID:  "pl-1",
		InputID:     "in-1",
		TriggeredBy: "user@example.com",
		Status:      pipeline.RunStatusPending,
		CreatedAt:   "2026-04-29T10:00:00Z",
		UpdatedAt:   "2026-04-29T10:00:00Z",
	}
}

func sampleLockedRun() pipeline.Run {
	r := sampleDraftRun()
	r.VersionID = intPtr(3)
	r.CovalentDispatchID = "cov-xyz"
	r.Status = pipeline.RunStatusRunning

	return r
}

func TestPrintRunJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintRunJSON(sampleDraftRun()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "d-1", parsed["run_id"])
	assert.Equal(t, "PENDING", parsed["status"])
	_, hasLegacy := parsed["dispatch_id"]
	assert.Falsef(t, hasLegacy, "expected legacy dispatch_id key to be absent")
}

func TestPrintRunHuman_DraftMissingCovalent(t *testing.T) {
	output := captureStdout(t, func() { PrintRunHuman(sampleDraftRun()) })
	assert.Contains(t, output, "Run ID:        d-1")
	assert.Contains(t, output, "Scope:         draft")
	assert.Contains(t, output, "Version:       \u2014")
	assert.Contains(t, output, "Covalent Run:  \u2014")
	assert.Contains(t, output, "Status:        PENDING")
}

func TestPrintRunHuman_LockedShowsErrorWhenSet(t *testing.T) {
	r := sampleLockedRun()
	r.Status = pipeline.RunStatusFailed
	r.ErrorDetail = "boom"

	output := captureStdout(t, func() { PrintRunHuman(r) })
	assert.Contains(t, output, "Scope:         locked")
	assert.Contains(t, output, "Version:       v3")
	assert.Contains(t, output, "Covalent Run:  cov-xyz")
	assert.Contains(t, output, "Error:         boom")
}

func TestPrintRunListJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintRunListJSON([]pipeline.Run{sampleDraftRun()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "d-1", parsed[0]["run_id"])
}

func TestPrintRunListHuman_Empty(t *testing.T) {
	output := captureStdout(t, func() { PrintRunListHuman(nil) })
	assert.Contains(t, output, "No runs found")
}

func TestPrintRunListHuman_RendersTable(t *testing.T) {
	output := captureStdout(t, func() {
		PrintRunListHuman([]pipeline.Run{sampleDraftRun(), sampleLockedRun()})
	})

	assert.Contains(t, output, "RUN_ID")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "TRIGGER")
	assert.Contains(t, output, "draft")
	assert.Contains(t, output, "locked")
	assert.Contains(t, output, "v3")
	assert.Contains(t, output, "PENDING")
	assert.Contains(t, output, "RUNNING")
}

func TestPrintStatusJSON(t *testing.T) {
	status := pipeline.RunStatus{
		RunID:              "d-1",
		Status:             pipeline.RunStatusCompleted,
		CovalentDispatchID: "cov-xyz",
	}

	output := captureStdout(t, func() {
		require.NoError(t, PrintStatusJSON(status))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "COMPLETED", parsed["status"])
	assert.Equal(t, "cov-xyz", parsed["covalent_run_id"])
	assert.Equal(t, "d-1", parsed["run_id"])
}

func TestPrintStatusHuman_NoCovalentRunID(t *testing.T) {
	output := captureStdout(t, func() {
		PrintStatusHuman(pipeline.RunStatus{RunID: "d-1", Status: "PENDING"})
	})

	assert.Contains(t, output, "Run ID:        d-1")
	assert.Contains(t, output, "Status:        PENDING")
	assert.Contains(t, output, "Covalent Run:  \u2014")
}
