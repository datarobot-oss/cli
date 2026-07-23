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

package workload

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

func makeTestArtifact(id, name, status, catalogID, versionID string) Artifact {
	a := Artifact{
		ID:        id,
		Name:      name,
		Status:    status,
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC),
	}

	if catalogID != "" {
		a.Spec = Spec{
			ContainerGroups: []ContainerGroup{
				{
					Containers: []Container{
						{
							ImageBuildConfig: &ImageBuildConfig{
								CodeRef: &CodeRef{
									Datarobot: &DatarobotCodeRef{
										CatalogID:        catalogID,
										CatalogVersionID: versionID,
									},
								},
							},
						},
					},
				},
			},
		}
	}

	return a
}

func TestPrintArtifactJSON_WithCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactJSON(artifact))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "art-abc-123", parsed["id"])
	assert.Equal(t, "my-agent", parsed["name"])
	assert.Equal(t, "DRAFT", parsed["status"])
	assert.Equal(t, "cat-xyz-789", parsed["catalogId"])
	assert.Equal(t, "fedcba09", parsed["versionId"])
	assert.Equal(t, "2026-04-01T08:00:00Z", parsed["createdAt"])
	assert.Equal(t, "2026-04-10T14:30:00Z", parsed["updatedAt"])
}

func TestPrintArtifactJSON_WithoutCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", "")

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactJSON(artifact))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Empty(t, parsed["catalogId"])
	assert.Empty(t, parsed["versionId"])
}

func TestPrintArtifactsJSON(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-001", "agent-one", "DRAFT", "cat-001", "ver-001"),
		makeTestArtifact("art-002", "agent-two", "LOCKED", "", ""),
	}

	output := captureStdout(t, func() {
		require.NoError(t, printArtifactsJSON(artifacts))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))

	items, ok := parsed["artifacts"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)

	item0 := items[0].(map[string]interface{})
	assert.Equal(t, "art-001", item0["id"])
	assert.Equal(t, "ver-001", item0["versionId"])

	item1 := items[1].(map[string]interface{})
	assert.Equal(t, "art-002", item1["id"])
	assert.Empty(t, item1["versionId"])
}

func TestPrintArtifactsJSON_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, printArtifactsJSON([]Artifact{}))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))

	items, ok := parsed["artifacts"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, items)
}

func TestPrintArtifactDetails_WithCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-xyz-789", "fedcba09")

	output := captureStdout(t, func() {
		printArtifactDetails(artifact)
	})

	assert.Contains(t, output, "art-abc-123")
	assert.Contains(t, output, "my-agent")
	assert.Contains(t, output, "DRAFT")
	assert.Contains(t, output, "cat-xyz-789")
	assert.Contains(t, output, "fedcba09")
	assert.Contains(t, output, "2026-04-01 08:00 UTC")
	assert.Contains(t, output, "2026-04-10 14:30 UTC")
	assert.Contains(t, output, "ID:")
	assert.Contains(t, output, "Catalog ID:")
	assert.Contains(t, output, "Version ID:")
}

func TestPrintArtifactDetails_WithoutCodeRef(t *testing.T) {
	artifact := makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", "")

	output := captureStdout(t, func() {
		printArtifactDetails(artifact)
	})

	assert.Contains(t, output, "Catalog ID:  —")
	assert.Contains(t, output, "Version ID:  —")
}

func TestPrintArtifactsTable_WithCodeRef(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "cat-001", "ver-001"),
	}

	output := captureStdout(t, func() {
		printArtifactsTable(artifacts)
	})

	assert.Contains(t, output, "ARTIFACT ID")
	assert.Contains(t, output, "CATALOG ID")
	assert.Contains(t, output, "VERSION ID")
	assert.Contains(t, output, "art-abc-123")
	assert.Contains(t, output, "my-agent")
	assert.Contains(t, output, "DRAFT")
	assert.Contains(t, output, "cat-001")
	assert.Contains(t, output, "ver-001")
	assert.Contains(t, output, "2026-04-10 14:30 UTC")
}

func TestPrintArtifactsTable_WithoutCodeRef(t *testing.T) {
	artifacts := []Artifact{
		makeTestArtifact("art-abc-123", "my-agent", "DRAFT", "", ""),
	}

	output := captureStdout(t, func() {
		printArtifactsTable(artifacts)
	})

	assert.Equal(t, 2, strings.Count(output, "—"))
}

func TestPrintArtifactsTable_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		printArtifactsTable([]Artifact{})
	})

	assert.Equal(t, "No artifacts found.\n", output)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr

	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()

	os.Stderr = old

	var buf bytes.Buffer

	_, _ = io.Copy(&buf, r)

	return buf.String()
}

func makeTestBuild(id, status string) Build {
	return Build{
		ID:         id,
		Name:       "image build",
		ArtifactID: "art-1",
		Status:     status,
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 9, 10, 0, 12, 0, time.UTC),
	}
}

func TestRenderBuild_Text(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuild(outputformat.OutputFormatText, makeTestBuild("b-1", BuildStatusCompleted)))
	})

	assert.Contains(t, out, "ID:")
	assert.Contains(t, out, "b-1")
	assert.Contains(t, out, "Status:")
	assert.Contains(t, out, BuildStatusCompleted)
}

func TestRenderBuild_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuild(outputformat.OutputFormatJSON, makeTestBuild("b-1", BuildStatusCompleted)))
	})

	var got map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "b-1", got["id"])
	assert.Equal(t, BuildStatusCompleted, got["status"])
}

func TestRenderBuilds_TextTable(t *testing.T) {
	builds := []Build{makeTestBuild("b-1", BuildStatusCompleted), makeTestBuild("b-2", BuildStatusFailed)}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuilds(outputformat.OutputFormatText, builds))
	})

	assert.Contains(t, out, "BUILD ID")
	assert.Contains(t, out, "b-1")
	assert.Contains(t, out, "b-2")
	assert.Contains(t, out, BuildStatusFailed)
}

func TestRenderBuilds_TextEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuilds(outputformat.OutputFormatText, nil))
	})

	assert.Equal(t, "No builds found.\n", out)
}

func TestRenderBuilds_JSONAlwaysArray(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuilds(outputformat.OutputFormatJSON, []Build{makeTestBuild("b-1", BuildStatusCompleted)}))
	})

	var got []map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "b-1", got[0]["id"])
}

func TestRenderBuildTrigger_TextOneIDPerLine(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildTrigger(outputformat.OutputFormatText, BuildTriggerResponse{BuildIDs: []string{"b-1", "b-2"}}))
	})

	assert.Equal(t, "b-1\nb-2\n", out, "exact one-id-per-line format is the BID=$(...) script contract")
}

func TestRenderBuildTrigger_JSONPassthrough(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildTrigger(outputformat.OutputFormatJSON, BuildTriggerResponse{BuildIDs: []string{"b-1"}}))
	})

	var got map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &got))
	ids, ok := got["buildIds"].([]any)
	require.True(t, ok)
	require.Len(t, ids, 1)
	assert.Equal(t, "b-1", ids[0])
}

func TestRenderBuildSummary_TextSuccessIncludesImage(t *testing.T) {
	summary := BuildSummary{
		BuildID:         "b-1",
		Status:          BuildStatusCompleted,
		DurationSeconds: 12,
		ImageURI:        "ecr/img:tag",
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildSummary(outputformat.OutputFormatText, summary))
	})

	assert.Equal(t, "Build b-1: COMPLETED in 12s (image: ecr/img:tag)\n", out)
}

func TestRenderBuildSummary_TextSuccessWithoutImageURI(t *testing.T) {
	summary := BuildSummary{
		BuildID:         "b-1",
		Status:          BuildStatusCompleted,
		DurationSeconds: 12,
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildSummary(outputformat.OutputFormatText, summary))
	})

	assert.Equal(t, "Build b-1: COMPLETED in 12s\n", out)
}

func TestRenderBuildSummary_TextFailureDumpsTailToStderr(t *testing.T) {
	summary := BuildSummary{
		BuildID:         "b-1",
		Status:          BuildStatusFailed,
		DurationSeconds: 9,
		LogTail: []BuildLogEntry{
			{Levelname: "INFO", Asctime: "t1", Message: "start"},
			{Levelname: "ERROR", Asctime: "t2", Message: "boom"},
		},
	}

	var (
		stdoutOut string
		stderrOut string
	)

	stderrOut = captureStderr(t, func() {
		stdoutOut = captureStdout(t, func() {
			require.NoError(t, RenderBuildSummary(outputformat.OutputFormatText, summary))
		})
	})

	assert.Equal(t, "Build b-1: FAILED in 9s\n", stdoutOut, "summary line stays on stdout")
	assert.Contains(t, stderrOut, "last 2 log lines")
	assert.Contains(t, stderrOut, "start")
	assert.Contains(t, stderrOut, "boom")
}

func TestRenderBuildSummary_JSONIncludesTail(t *testing.T) {
	summary := BuildSummary{
		BuildID:         "b-1",
		Status:          BuildStatusFailed,
		DurationSeconds: 9,
		LogTail: []BuildLogEntry{
			{Levelname: "ERROR", Message: "boom"},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildSummary(outputformat.OutputFormatJSON, summary))
	})

	var got map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, BuildStatusFailed, got["status"])
	assert.InEpsilon(t, float64(9), got["durationSeconds"], 0.001)

	tail, ok := got["logTail"].([]any)
	require.True(t, ok)
	require.Len(t, tail, 1)
}

func TestRenderBuildLogs_TextFormat(t *testing.T) {
	entries := []BuildLogEntry{
		{Levelname: "INFO", Asctime: "t1", Message: "started"},
		{Levelname: "ERROR", Asctime: "t2", Message: "broke"},
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildLogs(outputformat.OutputFormatText, entries))
	})

	assert.Contains(t, out, "[INFO] t1 started")
	assert.Contains(t, out, "[ERROR] t2 broke")
}

func TestRenderBuildLogs_JSONPassthroughPreservesRaw(t *testing.T) {
	entries := []BuildLogEntry{
		{
			Levelname: "INFO",
			Message:   "decoded",
			Raw:       json.RawMessage(`{"levelname":"INFO","extra":"server-field","message":"raw"}`),
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderBuildLogs(outputformat.OutputFormatJSON, entries))
	})

	var got []map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "server-field", got[0]["extra"], "raw bytes must pass through unchanged")
	assert.Equal(t, "raw", got[0]["message"])
}

func TestFilterLogsByLevel(t *testing.T) {
	entries := []BuildLogEntry{
		{Levelname: "DEBUG", Message: "d"},
		{Levelname: "INFO", Message: "i"},
		{Levelname: "WARNING", Message: "w"},
		{Levelname: "ERROR", Message: "e"},
		{Levelname: "UNKNOWN", Message: "u"},
	}

	t.Run("info drops DEBUG and keeps unknown", func(t *testing.T) {
		got := FilterLogsByLevel(entries, "info")

		msgs := make([]string, 0, len(got))

		for _, e := range got {
			msgs = append(msgs, e.Message)
		}

		assert.ElementsMatch(t, []string{"i", "w", "e", "u"}, msgs)
	})

	t.Run("debug keeps everything", func(t *testing.T) {
		got := FilterLogsByLevel(entries, "debug")
		assert.Len(t, got, len(entries))
	})

	t.Run("error keeps only ERROR and unknown", func(t *testing.T) {
		got := FilterLogsByLevel(entries, "error")

		msgs := make([]string, 0, len(got))

		for _, e := range got {
			msgs = append(msgs, e.Message)
		}

		assert.ElementsMatch(t, []string{"e", "u"}, msgs)
	})

	t.Run("unknown threshold is pass-through", func(t *testing.T) {
		got := FilterLogsByLevel(entries, "nonsense")
		assert.Equal(t, entries, got)
	})
}

func makeTestWorkload(id, name, status string) Workload {
	return Workload{
		ID:         id,
		Name:       name,
		Status:     status,
		Type:       "service",
		Importance: "low",
		ArtifactID: "art-1",
		Endpoint:   "https://app.example.com/api/v2/endpoints/workloads/" + id + "/",
		CreatedAt:  time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 10, 8, 5, 0, 0, time.UTC),
	}
}

func TestPrintWorkloadJSON(t *testing.T) {
	workload := makeTestWorkload("wl-1", "my-app", "running")

	output := captureStdout(t, func() {
		require.NoError(t, printWorkloadJSON(workload))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "wl-1", parsed["id"])
	assert.Equal(t, "my-app", parsed["name"])
	assert.Equal(t, "running", parsed["status"])
	assert.Equal(t, "service", parsed["type"])
	assert.Equal(t, "low", parsed["importance"])
	assert.Equal(t, "art-1", parsed["artifactId"])
	assert.Equal(t, "https://app.example.com/api/v2/endpoints/workloads/wl-1/", parsed["endpoint"])
	assert.Equal(t, "2026-06-10T08:00:00Z", parsed["createdAt"])
	assert.Equal(t, "2026-06-10T08:05:00Z", parsed["updatedAt"])
	// The projection must not leak server-side extras.
	assert.NotContains(t, parsed, "owners")
	assert.NotContains(t, parsed, "permissions")
}

func TestPrintWorkloadsJSON(t *testing.T) {
	workloads := []Workload{
		makeTestWorkload("wl-1", "app-one", "running"),
		makeTestWorkload("wl-2", "app-two", "errored"),
	}

	output := captureStdout(t, func() {
		require.NoError(t, printWorkloadsJSON(workloads))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))

	items, ok := parsed["workloads"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)

	item0 := items[0].(map[string]interface{})
	assert.Equal(t, "wl-1", item0["id"])

	item1 := items[1].(map[string]interface{})
	assert.Equal(t, "errored", item1["status"])
}

func TestPrintWorkloadsJSON_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, printWorkloadsJSON(nil))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(output), &parsed))

	items, ok := parsed["workloads"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, items)
}

func TestPrintWorkloadDetails(t *testing.T) {
	workload := makeTestWorkload("wl-1", "my-app", "launching")

	output := captureStdout(t, func() {
		printWorkloadDetails(workload)
	})

	assert.Contains(t, output, "ID:")
	assert.Contains(t, output, "wl-1")
	assert.Contains(t, output, "Status:")
	assert.Contains(t, output, "launching")
	assert.Contains(t, output, "Endpoint:")
	assert.Contains(t, output, "https://app.example.com/api/v2/endpoints/workloads/wl-1/")
	assert.Contains(t, output, "Artifact ID:")
	assert.Contains(t, output, "2026-06-10 08:00 UTC")

	// Endpoint must come right after Status so repeated `workload get`
	// polling reads naturally during the deploy loop.
	statusIdx := strings.Index(output, "Status:")
	endpointIdx := strings.Index(output, "Endpoint:")

	require.GreaterOrEqual(t, statusIdx, 0)
	assert.Greater(t, endpointIdx, statusIdx)
}

func TestPrintWorkloadDetails_EmptyEndpointPlaceholder(t *testing.T) {
	workload := makeTestWorkload("wl-1", "my-app", "submitted")
	workload.Endpoint = ""

	output := captureStdout(t, func() {
		printWorkloadDetails(workload)
	})

	assert.Contains(t, output, "—")
}

func TestPrintWorkloadsTable(t *testing.T) {
	workloads := []Workload{
		makeTestWorkload("wl-1", "app-one", "running"),
		makeTestWorkload("wl-2", "app-two", "stopped"),
	}

	output := captureStdout(t, func() {
		printWorkloadsTable(workloads)
	})

	assert.Contains(t, output, "WORKLOAD ID")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "IMPORTANCE")
	assert.Contains(t, output, "wl-1")
	assert.Contains(t, output, "app-two")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "stopped")
	assert.Contains(t, output, "2026-06-10 08:05 UTC")
}

func TestPrintWorkloadsTable_Empty(t *testing.T) {
	output := captureStdout(t, func() {
		printWorkloadsTable([]Workload{})
	})

	assert.Equal(t, "No workloads found.\n", output)
}

func TestRenderWorkloadOperation_TextPrintsServerMessage(t *testing.T) {
	resp := WorkloadOperationResponse{
		Status:     "Proton is already stopped",
		WorkloadID: "wl-1",
		TrackVia:   "/api/v2/workloads/wl-1",
	}

	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadOperation(outputformat.OutputFormatText, resp))
	})

	assert.Equal(t, "Proton is already stopped\n", output)
}

func TestRenderWorkloadOperation_JSON(t *testing.T) {
	resp := WorkloadOperationResponse{
		Status:     "started",
		WorkloadID: "wl-1",
		TrackVia:   "/api/v2/workloads/wl-1",
	}

	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadOperation(outputformat.OutputFormatJSON, resp))
	})

	assert.JSONEq(t,
		`{"status": "started", "workloadId": "wl-1", "trackVia": "/api/v2/workloads/wl-1"}`,
		output)
}

func TestRenderWorkloadStatus_TextBare(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadStatus(outputformat.OutputFormatText, makeTestWorkload("wl-1", "a", "running")))
	})

	// The bare status value is the script-capture contract.
	assert.Equal(t, "running\n", output)
}

func TestRenderWorkloadStatus_JSONShape(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadStatus(outputformat.OutputFormatJSON, makeTestWorkload("wl-1", "a", "errored")))
	})

	assert.JSONEq(t, `{"id": "wl-1", "status": "errored"}`, output)
}

func makeTestLogEntry(ts, level, message string) WorkloadLogEntry {
	return WorkloadLogEntry{Timestamp: ts, Level: level, Message: message}
}

func TestRenderWorkloadLogs_TextFormat(t *testing.T) {
	entries := []WorkloadLogEntry{
		makeTestLogEntry("2026-06-11 14:04:14+00:00", "info", "started"),
		makeTestLogEntry("2026-06-11 14:04:15+00:00", "error", "boom"),
	}

	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadLogs(outputformat.OutputFormatText, entries))
	})

	assert.Contains(t, output, "[INFO] 2026-06-11 14:04:14+00:00 started")
	assert.Contains(t, output, "[ERROR] 2026-06-11 14:04:15+00:00 boom")
}

func TestRenderWorkloadLogs_TextEmpty(t *testing.T) {
	var stderr string

	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			require.NoError(t, RenderWorkloadLogs(outputformat.OutputFormatText, nil))
		})
	})

	// The hint goes to stderr so stdout stays log lines only (pipe/grep safe).
	assert.Empty(t, stdout)
	assert.Equal(t, "No logs found.\n", stderr)
}

func TestRenderWorkloadLogs_JSONAlwaysArray(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadLogs(outputformat.OutputFormatJSON, nil))
	})

	// A regression that emits `null` instead of a JSON array must fail here.
	assert.JSONEq(t, `[]`, output)
}

func TestRenderWorkloadLogs_JSONPassthrough(t *testing.T) {
	entries := []WorkloadLogEntry{makeTestLogEntry("2026-06-11 14:04:14+00:00", "INFO", "hi")}

	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadLogs(outputformat.OutputFormatJSON, entries))
	})

	assert.JSONEq(t,
		`[{"timestamp": "2026-06-11 14:04:14+00:00", "level": "INFO", "message": "hi"}]`,
		output)
}

func TestRenderWorkloadLogLine_Text(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadLogLine(outputformat.OutputFormatText,
			makeTestLogEntry("2026-06-11 14:04:14+00:00", "info", "hello")))
	})

	assert.Equal(t, "[INFO] 2026-06-11 14:04:14+00:00 hello\n", output)
}

func TestRenderWorkloadLogLine_JSONLineCompact(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderWorkloadLogLine(outputformat.OutputFormatJSON,
			makeTestLogEntry("2026-06-11 14:04:14+00:00", "INFO", "hello")))
	})

	line := strings.TrimSuffix(output, "\n")

	// One JSON object on a single line (JSON Lines): the absence of any
	// internal newline is what distinguishes it from indented output.
	assert.NotContains(t, line, "\n")
	assert.JSONEq(t,
		`{"timestamp": "2026-06-11 14:04:14+00:00", "level": "INFO", "message": "hello"}`,
		line)
}

func TestEnvironmentVarSourceLabel(t *testing.T) {
	assert.Equal(t, "plain", environmentVarSourceLabel(EnvironmentVar{Name: "LOG_LEVEL", Value: "debug"}))
	assert.Equal(t, "dr-credential", environmentVarSourceLabel(EnvironmentVar{
		Source: EnvironmentVarSourceDRCredential, Name: "API_KEY",
	}))
}

func TestEnvironmentVarDisplayValue(t *testing.T) {
	t.Run("plain var shows its value", func(t *testing.T) {
		value := environmentVarDisplayValue(EnvironmentVar{Name: "LOG_LEVEL", Value: "debug"})
		assert.Equal(t, "debug", value)
	})

	t.Run("credential-backed var never shows a value", func(t *testing.T) {
		value := environmentVarDisplayValue(EnvironmentVar{
			Source:         EnvironmentVarSourceDRCredential,
			Name:           "API_KEY",
			DRCredentialID: "cred-1",
			Key:            "apiToken",
		})
		assert.Equal(t, "dr-credential:cred-1/apiToken", value)
	})
}

func TestRenderEnvironmentVars_Text(t *testing.T) {
	vars := []EnvironmentVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Source: EnvironmentVarSourceDRCredential, Name: "API_KEY", DRCredentialID: "cred-1", Key: "apiToken"},
	}

	output := captureStdout(t, func() {
		require.NoError(t, RenderEnvironmentVars(outputformat.OutputFormatText, vars))
	})

	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "SOURCE")
	assert.Contains(t, output, "VALUE")
	assert.Contains(t, output, "LOG_LEVEL")
	assert.Contains(t, output, "plain")
	assert.Contains(t, output, "debug")
	assert.Contains(t, output, "API_KEY")
	assert.Contains(t, output, "dr-credential")
	assert.Contains(t, output, "cred-1/apiToken")
}

func TestRenderEnvironmentVars_TextEmpty(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderEnvironmentVars(outputformat.OutputFormatText, nil))
	})

	assert.Equal(t, "No environment variables set.\n", output)
}

func TestRenderEnvironmentVars_JSONIncludesPlainValueButNotCredentialSecret(t *testing.T) {
	vars := []EnvironmentVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Source: EnvironmentVarSourceDRCredential, Name: "API_KEY", DRCredentialID: "cred-1", Key: "apiToken"},
	}

	output := captureStdout(t, func() {
		require.NoError(t, RenderEnvironmentVars(outputformat.OutputFormatJSON, vars))
	})

	assert.JSONEq(t, `[
		{"name": "LOG_LEVEL", "value": "debug"},
		{"source": "dr-credential", "name": "API_KEY", "drCredentialId": "cred-1", "key": "apiToken"}
	]`, output)
}

func TestRenderEnvironmentVars_JSONEmptyIsArrayNotNull(t *testing.T) {
	output := captureStdout(t, func() {
		require.NoError(t, RenderEnvironmentVars(outputformat.OutputFormatJSON, nil))
	})

	assert.JSONEq(t, `[]`, output)
}
