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

package clone

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/datarobot/cli/cmd/pipeline/internal/testutil"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sample is a clone response: always a fresh draft, so Version/Status are
// intentionally zero-valued (the server returns none for a draft, same as
// POST /pipelines).
func sample() pipeline.CreateResponse {
	return pipeline.CreateResponse{
		PipelineID: "p-2",
		Name:       "Clone of wf",
		Mode:       "draft",
		TaskNames:  []string{"e1", "e2"},
		CreatedAt:  time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
	}
}

func TestPrintCloneJSON(t *testing.T) {
	output := testutil.CaptureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(outputformat.OutputFormatJSON, sample()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "p-2", parsed["id"])
	assert.Equal(t, "Clone of wf", parsed["name"])
	assert.Equal(t, "draft", parsed["mode"])
}

func TestPrintCloneHuman(t *testing.T) {
	output := testutil.CaptureStdout(t, func() {
		require.NoError(t, pipeline.RenderCreateResponse(outputformat.OutputFormatText, sample()))
	})

	assert.Contains(t, output, "Clone of wf")
	assert.Contains(t, output, "draft")
	assert.Contains(t, output, "e1, e2")
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

func TestCmd_RejectsBlankName(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{"abc", "--name", "   "})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.PreRunE = nil

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name must not be blank")
}
