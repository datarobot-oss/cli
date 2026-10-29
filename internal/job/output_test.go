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

package job

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	prev := out
	out = &buf

	t.Cleanup(func() { out = prev })

	return &buf
}

func failedRun() Run {
	started := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	finished := started.Add(30 * time.Minute)
	code := 137

	return Run{
		ID:                 "run-1",
		JobID:              "job-1",
		Status:             StatusFailed,
		Placement:          &Placement{EnclaveID: "uuid-east", Enclave: "prod-east"},
		StartedAt:          &started,
		FinishedAt:         &finished,
		ExitCode:           &code,
		TerminationReason:  "OOMKilled",
		TerminationMessage: "out of memory",
		CreatedAt:          started,
	}
}

func TestRenderRun_ShowsHowItEnded(t *testing.T) {
	buf := capture(t)

	require.NoError(t, RenderRun(outputformat.OutputFormatText, failedRun()))

	text := buf.String()
	assert.Contains(t, text, "failed")
	assert.Contains(t, text, "Enclave prod-east")
	assert.Contains(t, text, "137")
	assert.Contains(t, text, "OOMKilled")
	assert.Contains(t, text, "out of memory")
	assert.Contains(t, text, "2026-10-28 12:30:00 UTC")
}

func TestRenderRun_AnActiveRunOnTheHubHasNoExitYet(t *testing.T) {
	buf := capture(t)

	require.NoError(t, RenderRun(outputformat.OutputFormatText, Run{ID: "run-1", Status: StatusRunning}))

	text := buf.String()
	assert.Contains(t, text, "Hub")
	assert.NotContains(t, text, "Exit code")
}

func TestRenderRun_JSONKeepsTheAPIShape(t *testing.T) {
	buf := capture(t)

	require.NoError(t, RenderRun(outputformat.OutputFormatJSON, failedRun()))

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "failed", decoded["status"])
	assert.InDelta(t, 137, decoded["exitCode"], 0)
	assert.Equal(t, "job-1", decoded["jobId"])
}

func TestRenderRuns_EmptyJSONIsAnEmptyList(t *testing.T) {
	buf := capture(t)

	require.NoError(t, RenderRuns(outputformat.OutputFormatJSON, nil))

	assert.Contains(t, buf.String(), `"runs": []`)
}

func TestRenderJobs_Table(t *testing.T) {
	buf := capture(t)

	require.NoError(t, RenderJobs(outputformat.OutputFormatText, []Job{{ID: "job-1", Name: "train", ArtifactID: "art-1"}}))

	assert.Contains(t, buf.String(), "job-1")
	assert.Contains(t, buf.String(), "train")
}
