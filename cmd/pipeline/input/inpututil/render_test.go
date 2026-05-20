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

package inpututil

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

func sampleDraftInput() pipeline.Input {
	return pipeline.Input{
		InputID:    "in-1",
		PipelineID: "pl-1",
		IsDraft:    true,
		State:      pipeline.InputStateValid,
		Payload:    map[string]any{"k": "v"},
		CreatedAt:  "2026-04-29T10:00:00Z",
		UpdatedAt:  "2026-04-29T10:05:00Z",
	}
}

func sampleLockedInput() pipeline.Input {
	in := sampleDraftInput()
	in.IsDraft = false
	in.VersionID = intPtr(2)

	return in
}

func TestPrintInputJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintInputJSON(sampleDraftInput()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "in-1", parsed["input_id"])
	assert.Equal(t, "VALID", parsed["state"])
}

func TestPrintInputHuman_Draft(t *testing.T) {
	output := captureStdout(t, func() { PrintInputHuman(sampleDraftInput()) })
	assert.Contains(t, output, "Input ID:    in-1")
	assert.Contains(t, output, "Scope:       draft")
	assert.Contains(t, output, "Version:     \u2014")
	assert.Contains(t, output, "State:       VALID")
	assert.Contains(t, output, "Payload:")
	assert.Contains(t, output, `"k": "v"`)
}

func TestPrintInputHuman_Locked(t *testing.T) {
	output := captureStdout(t, func() { PrintInputHuman(sampleLockedInput()) })
	assert.Contains(t, output, "Scope:       locked")
	assert.Contains(t, output, "Version:     v2")
}

func TestPrintInputListJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintInputListJSON([]pipeline.Input{sampleDraftInput()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "in-1", parsed[0]["input_id"])
}

func TestPrintInputListHuman_Empty(t *testing.T) {
	output := captureStdout(t, func() { PrintInputListHuman(nil) })
	assert.Contains(t, output, "No inputs found")
}

func TestPrintInputListHuman_RendersTable(t *testing.T) {
	output := captureStdout(t, func() {
		PrintInputListHuman([]pipeline.Input{sampleDraftInput(), sampleLockedInput()})
	})

	assert.Contains(t, output, "INPUT_ID")
	assert.Contains(t, output, "SCOPE")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "draft")
	assert.Contains(t, output, "locked")
	assert.Contains(t, output, "v2")
	assert.Contains(t, output, "VALID")
}
