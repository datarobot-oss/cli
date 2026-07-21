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

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleTask() PipelineTask {
	ver := 2
	ann := "int"

	return PipelineTask{
		TaskID:     1,
		PipelineID: "p-1",
		VersionID:  &ver,
		Name:       "add",
		Parameters: []TaskParameter{
			{Name: "x", Annotation: &ann},
			{Name: "y", Annotation: &ann},
		},
		Inputs: map[string]any{"a": float64(100)},
		Source: "def add(x: int, y: int) -> int:\n    return x + y",
	}
}

// ── toTaskJSON ───────────────────────────────────────────────────────────────

func TestToTaskJSON_RemapsWireFields(t *testing.T) {
	j := toTaskJSON(sampleTask())

	assert.Equal(t, 1, j.TaskID)
	assert.Equal(t, "p-1", j.PipelineID)
	require.NotNil(t, j.VersionID)
	assert.Equal(t, 2, *j.VersionID)
	assert.Equal(t, "add", j.Name)
	require.Len(t, j.Parameters, 2)
	assert.Equal(t, "x", j.Parameters[0].Name)
}

func TestToTaskJSON_DraftScope(t *testing.T) {
	tk := sampleTask()
	tk.VersionID = nil
	tk.Inputs = nil

	j := toTaskJSON(tk)

	assert.Nil(t, j.VersionID)
	assert.Nil(t, j.Inputs)
}

func TestToTaskJSON_JSONKeysUseCliVocabulary(t *testing.T) {
	data, err := json.Marshal(toTaskJSON(sampleTask()))
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "task_id")
	assert.Contains(t, raw, "pipeline_id")
	assert.Contains(t, raw, "version_id")
	assert.NotContains(t, raw, "id", "wire 'id' must not appear in CLI output")
	assert.NotContains(t, raw, "pipelineId")
	assert.NotContains(t, raw, "versionId")
	assert.NotContains(t, raw, "resourceBundle", "camelCase wire key must not appear in CLI output")
	assert.NotContains(t, raw, "taskGroupId", "camelCase wire key must not appear in CLI output")
}

func TestToTaskJSON_NewFieldsOmittedWhenNil(t *testing.T) {
	tk := sampleTask()
	tk.ResourceBundle = nil
	tk.TaskGroupID = nil

	data, err := json.Marshal(toTaskJSON(tk))
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "resource_bundle", "omitempty: nil ResourceBundle must be absent")
	assert.NotContains(t, raw, "task_group_id", "omitempty: nil TaskGroupID must be absent")
}

func TestToTaskJSON_NewFieldsPassedThrough(t *testing.T) {
	grp := 7
	tk := sampleTask()
	tk.ResourceBundle = map[string]any{"cpu": "2", "memory": "4Gi"}
	tk.TaskGroupID = &grp

	j := toTaskJSON(tk)

	require.NotNil(t, j.ResourceBundle)
	assert.Equal(t, "2", j.ResourceBundle["cpu"])
	require.NotNil(t, j.TaskGroupID)
	assert.Equal(t, 7, *j.TaskGroupID)
}

// ── RenderTask ───────────────────────────────────────────────────────────────

func TestRenderTask_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderTask(outputformat.OutputFormatJSON, sampleTask()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.EqualValues(t, 1, parsed["task_id"])
	assert.Equal(t, "add", parsed["name"])
	assert.Equal(t, "def add(x: int, y: int) -> int:\n    return x + y", parsed["source"])
}

func TestRenderTask_Human(t *testing.T) {
	out := captureStdout(t, func() { printTaskHuman(sampleTask()) })

	assert.Contains(t, out, "Task ID:")
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, "add")
	assert.Contains(t, out, "x")
	assert.Contains(t, out, "def add")
}

func TestPrintTaskHuman_DraftNoInputs(t *testing.T) {
	tk := sampleTask()
	tk.VersionID = nil
	tk.Inputs = nil

	out := captureStdout(t, func() { printTaskHuman(tk) })

	assert.Contains(t, out, "draft")
	assert.NotContains(t, out, "Inputs:")
}
