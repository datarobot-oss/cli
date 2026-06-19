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

func sampleInput() Input {
	ver := 3

	return Input{
		InputID:    "in-1",
		PipelineID: "p-1",
		VersionID:  &ver,
		IsDraft:    false,
		Payload:    map[string]any{"key": "value"},
		State:      InputStateValid,
		CreatedAt:  time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 29, 10, 5, 0, 0, time.UTC),
	}
}

// ── toInputJSON remapping ────────────────────────────────────────────────────

func TestToInputJSON_RemapsWireFields(t *testing.T) {
	j := toInputJSON(sampleInput())

	assert.Equal(t, "in-1", j.InputID)
	assert.Equal(t, "p-1", j.PipelineID)
	assert.Equal(t, "locked", j.Scope)
	require.NotNil(t, j.VersionID)
	assert.Equal(t, 3, *j.VersionID)
	assert.Equal(t, string(InputStateValid), j.State)
	assert.Equal(t, map[string]any{"key": "value"}, j.Payload)
}

func TestToInputJSON_DraftScope(t *testing.T) {
	in := sampleInput()
	in.VersionID = nil

	j := toInputJSON(in)

	assert.Equal(t, "draft", j.Scope)
	assert.Nil(t, j.VersionID)
}

func TestToInputJSON_FormatsTimestampsAsRFC3339(t *testing.T) {
	j := toInputJSON(sampleInput())

	assert.Equal(t, "2026-04-29T10:00:00Z", j.CreatedAt)
	assert.Equal(t, "2026-04-29T10:05:00Z", j.UpdatedAt)
}

func TestToInputJSON_JSONKeysUseCliVocabulary(t *testing.T) {
	data, err := json.Marshal(toInputJSON(sampleInput()))
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "input_id", "wire 'id' must be remapped to 'input_id'")
	assert.Contains(t, raw, "pipeline_id")
	assert.Contains(t, raw, "scope")
	assert.Contains(t, raw, "version_id")
	assert.NotContains(t, raw, "id", "raw wire key 'id' must not appear in CLI output")
	assert.NotContains(t, raw, "pipelineId")
	assert.NotContains(t, raw, "versionId")
}

// ── RenderInput ──────────────────────────────────────────────────────────────

func TestRenderInput_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderInput(outputformat.OutputFormatJSON, sampleInput()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, "in-1", parsed["input_id"])
	assert.Equal(t, "locked", parsed["scope"])
	assert.Equal(t, "2026-04-29T10:00:00Z", parsed["created_at"])
}

func TestRenderInput_Human(t *testing.T) {
	out := captureStdout(t, func() { PrintInputHuman(sampleInput()) })

	assert.Contains(t, out, "in-1")
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, string(InputStateValid))
}

// ── PrintInputListJSON ───────────────────────────────────────────────────────

func TestPrintInputListJSON_RemapsFields(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, PrintInputListJSON([]Input{sampleInput()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "in-1", parsed[0]["input_id"])
	assert.Equal(t, "locked", parsed[0]["scope"])
}

// ── PrintInputListHuman ──────────────────────────────────────────────────────

func TestPrintInputListHuman_Empty(t *testing.T) {
	out := captureStdout(t, func() { PrintInputListHuman(nil) })
	assert.Contains(t, out, "No inputs found")
}

func TestPrintInputListHuman_RendersTable(t *testing.T) {
	out := captureStdout(t, func() { PrintInputListHuman([]Input{sampleInput()}) })

	assert.Contains(t, out, "INPUT ID")
	assert.Contains(t, out, "in-1")
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, string(InputStateValid))
}
