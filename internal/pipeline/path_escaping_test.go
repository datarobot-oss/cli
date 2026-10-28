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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evilID contains the three characters that let a caller-supplied id rewrite
// the request: "/" adds a path segment, ".." climbs one, and "?" moves the
// remainder of the path into the query string.
const evilID = "a/b?c/../x"

// escapedEvilID is what evilID must look like on the wire.
const escapedEvilID = "a%2Fb%3Fc%2F..%2Fx"

// assertSingleSegment checks the RAW request target -- r.RequestURI, i.e. the
// bytes the client actually sent.
//
// It deliberately does not assert on r.URL.Path: that field is the *decoded*
// path, so a correctly-encoded %2F reads back as a plain "/" there and the
// assertion would fail on working code. Routers, proxies and path-normalizers
// dispatch on the raw target, so the raw target is where the security property
// lives.
func assertSingleSegment(t *testing.T, rawURI string) {
	t.Helper()

	// Split off any query string before reasoning about segments.
	rawPath, rawQuery, _ := strings.Cut(rawURI, "?")

	assert.Contains(t, rawPath, escapedEvilID,
		"the id must be percent-encoded into a single segment; raw target was %q", rawURI)
	assert.NotContains(t, rawPath, "/../",
		"the id must not introduce a traversal segment; raw target was %q", rawURI)
	assert.NotContains(t, rawQuery, "..",
		"no part of the id may land in the query string; raw target was %q", rawURI)

	// The encoded id appears exactly once and contributes exactly one segment.
	parts := strings.Split(rawPath, escapedEvilID)
	require.Len(t, parts, 2, "expected the encoded id exactly once in %q", rawPath)
	assert.NotContains(t, parts[0], "?", "nothing before the id may open a query")
}

// captureServer records the raw request target of the single request made to it.
func captureServer(t *testing.T, body string) (*httptest.Server, *string) {
	t.Helper()

	var gotRawURI string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawURI = r.RequestURI

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv, &gotRawURI
}

// TestURLBuilders_EscapeResourceIDs is the regression guard for the pre-GA
// security review finding: every pipelines client path builder must
// percent-encode caller-supplied resource ids so a hostile id cannot rewrite
// the authenticated request.
func TestURLBuilders_EscapeResourceIDs(t *testing.T) {
	v := 1

	cases := []struct {
		name string
		body string
		call func(t *testing.T)
	}{
		// scope.go -- PipelinePath, shared by inputs/runs/graph/source/task
		{"GetGraph", `{}`, func(t *testing.T) { _, _ = GetGraph(evilID, ScopeDraft, nil) }},
		{"GetPipelineSource", `{}`, func(t *testing.T) { _, _ = GetPipelineSource(evilID, ScopeDraft, nil) }},
		{"GetTask", `{}`, func(t *testing.T) { _, _ = GetTask(evilID, ScopeDraft, nil, 1) }},
		{"GetGraph_locked", `{}`, func(t *testing.T) { _, _ = GetGraph(evilID, ScopeLocked, &v) }},

		// input.go -- id in the PipelinePath suffix
		{"CreateInput", `{}`, func(t *testing.T) { _, _ = CreateInput(evilID, ScopeDraft, nil, nil) }},
		{"ListInputs", `{"data":[]}`, func(t *testing.T) { _, _ = ListInputs(evilID, ScopeDraft, nil, 0, 0) }},
		{"GetInput", `{}`, func(t *testing.T) { _, _ = GetInput("p-1", ScopeDraft, nil, evilID) }},
		{"UpdateInput", `{}`, func(t *testing.T) { _, _ = UpdateInput("p-1", evilID, nil) }},
		{"DeleteInput", `{}`, func(t *testing.T) { _ = DeleteInput("p-1", ScopeDraft, nil, evilID) }},

		// run.go -- id in the PipelinePath suffix
		{"CreateRun", `{}`, func(t *testing.T) { _, _ = CreateRun(evilID, ScopeDraft, nil, "i", "im") }},
		{"ListRuns", `{"data":[]}`, func(t *testing.T) { _, _ = ListRuns(evilID, ScopeDraft, nil, 0, 0) }},
		{"GetRun", `{}`, func(t *testing.T) { _, _ = GetRun("p-1", ScopeDraft, nil, evilID) }},
		{"GetRunStatus", `{}`, func(t *testing.T) { _, _ = GetRunStatus("p-1", ScopeDraft, nil, evilID) }},

		// run_task.go -- taskBase, both ids
		{"ListTaskExecutions_pipeline", `{"data":[]}`, func(t *testing.T) { _, _ = ListTaskExecutions(evilID, "r-1") }},
		{"ListTaskExecutions_run", `{"data":[]}`, func(t *testing.T) { _, _ = ListTaskExecutions("p-1", evilID) }},
		{"GetTaskExecution", `{}`, func(t *testing.T) { _, _ = GetTaskExecution("p-1", evilID, 1, nil) }},
		{"GetTaskLogs", `{}`, func(t *testing.T) { _, _ = GetTaskLogs("p-1", evilID, 1, nil, nil, "user") }},
		{"GetTaskResult", `{}`, func(t *testing.T) { _, _ = GetTaskResult("p-1", evilID, 1, nil) }},
		{"GetTaskDurableLog", `{}`, func(t *testing.T) {
			_, _ = GetTaskDurableLog("p-1", evilID, 1, nil, "stdout", "user")
		}},

		// schedule.go -- scheduleBase and the per-verb id append
		{"CreateSchedule", `{}`, func(t *testing.T) { _, _ = CreateSchedule(evilID, ScheduleCreateRequest{}) }},
		{"ListSchedules", `{"data":[]}`, func(t *testing.T) { _, _ = ListSchedules(evilID, 0, 0) }},
		{"GetSchedule", `{}`, func(t *testing.T) { _, _ = GetSchedule("p-1", evilID) }},
		{"UpdateSchedule", `{}`, func(t *testing.T) { _, _ = UpdateSchedule("p-1", evilID, ScheduleUpdateRequest{}) }},
		{"DeleteSchedule", `{}`, func(t *testing.T) { _ = DeleteSchedule("p-1", evilID) }},

		// version.go
		{"ListVersions", `{"data":[]}`, func(t *testing.T) { _, _ = ListVersions(evilID, 0, 0) }},
		{"GetVersion", `{}`, func(t *testing.T) { _, _ = GetVersion(evilID, 1) }},

		// image.go
		{"GetImage", `{}`, func(t *testing.T) { _, _ = GetImage(evilID) }},
		{"GetImageBuildLogs", `{}`, func(t *testing.T) { _, _ = GetImageBuildLogs(evilID, 1) }},
		{"UpdateImage", `{}`, func(t *testing.T) {
			_, _ = UpdateImage(evilID, nil, nil, "", "", false)
		}},
		{"DeleteImage", `{}`, func(t *testing.T) { _ = DeleteImage(evilID) }},
		{"DeleteImageVersion", `{}`, func(t *testing.T) { _ = DeleteImageVersion(evilID, 1) }},

		// pipeline.go -- already escaped before this change; pinned so it stays that way
		{"GetPipeline", `{}`, func(t *testing.T) { _, _ = GetPipeline(evilID) }},
		{"DeletePipeline", `{}`, func(t *testing.T) { _ = DeletePipeline(evilID) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installSkipAuth(t)

			srv, gotRawURI := captureServer(t, tc.body)
			installEndpoint(t, srv.URL)

			tc.call(t)

			require.NotEmpty(t, *gotRawURI, "no request reached the server")
			assertSingleSegment(t, *gotRawURI)
		})
	}
}

// TestEscapeID_EncodesReservedCharacters pins the primitive itself.
func TestEscapeID_EncodesReservedCharacters(t *testing.T) {
	assert.Equal(t, "a%2Fb%3Fc", escapeID("a/b?c"))
	assert.Equal(t, "..%2Fp-1", escapeID("../p-1"))
	assert.Equal(t, "p-1", escapeID("p-1"), "safe ids must pass through unchanged")
}

// TestPipelinePath_DoesNotEscapeSuffix documents the split contract: the
// builder escapes its own pipelineID but leaves the suffix alone, so callers
// interpolating an id into a suffix must escape it themselves.
func TestPipelinePath_DoesNotEscapeSuffix(t *testing.T) {
	got, err := PipelinePath("a/b", ScopeDraft, nil, "inputs/c/d")
	require.NoError(t, err)
	assert.Equal(t, "/api/v2/pipelines/a%2Fb/inputs/c/d", got,
		"pipelineID escaped, suffix passed through verbatim")
}
