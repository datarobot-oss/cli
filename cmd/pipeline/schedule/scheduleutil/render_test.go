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

package scheduleutil

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

func sampleSchedule() pipeline.Schedule {
	return pipeline.Schedule{
		ScheduleID:     "s-1",
		PipelineID:     "pl-1",
		Version:        2,
		CronExpression: "0 * * * *",
		Timezone:       "UTC",
		Status:         pipeline.ScheduleStatusActive,
		CreatedAt:      "2026-04-29T10:00:00Z",
		UpdatedAt:      "2026-04-29T11:00:00Z",
	}
}

func TestPrintScheduleJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintScheduleJSON(sampleSchedule()))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "s-1", parsed["schedule_id"])
	assert.Equal(t, "ACTIVE", parsed["status"])
	assert.EqualValues(t, 2, parsed["version"])
}

func TestPrintScheduleHuman(t *testing.T) {
	output := captureStdout(t, func() { PrintScheduleHuman(sampleSchedule()) })
	assert.Contains(t, output, "Schedule ID:  s-1")
	assert.Contains(t, output, "Version:      v2")
	assert.Contains(t, output, "Cron:         0 * * * *")
	assert.Contains(t, output, "Timezone:     UTC")
	assert.Contains(t, output, "Status:       ACTIVE")
}

func TestPrintScheduleListJSON(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, PrintScheduleListJSON([]pipeline.Schedule{sampleSchedule()}))
	})

	var parsed []map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "s-1", parsed[0]["schedule_id"])
}

func TestPrintScheduleListHuman_Empty(t *testing.T) {
	output := captureStdout(t, func() { PrintScheduleListHuman(nil) })
	assert.Contains(t, output, "No schedules found")
}

func TestPrintScheduleListHuman_RendersTable(t *testing.T) {
	output := captureStdout(t, func() {
		PrintScheduleListHuman([]pipeline.Schedule{sampleSchedule()})
	})

	assert.Contains(t, output, "SCHEDULE_ID")
	assert.Contains(t, output, "VERSION")
	assert.Contains(t, output, "CRON")
	assert.Contains(t, output, "TIMEZONE")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "s-1")
	assert.Contains(t, output, "v2")
	assert.Contains(t, output, "0 * * * *")
	assert.Contains(t, output, "UTC")
	assert.Contains(t, output, "ACTIVE")
}
