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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTerminalBuildStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{BuildStatusPending, false},
		{BuildStatusInProgress, false},
		{BuildStatusCompleted, true},
		{BuildStatusFailed, true},
		{BuildStatusCancelled, true},
		{"UNKNOWN", false},
		{"", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsTerminalBuildStatus(c.status), "status %q", c.status)
	}
}

func TestIsBuildErrorStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{BuildStatusCompleted, false},
		{BuildStatusFailed, true},
		{BuildStatusCancelled, true},
		{BuildStatusPending, false},
		{"WHATEVER", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsBuildErrorStatus(c.status), "status %q", c.status)
	}
}

func TestBuildDurationSeconds(t *testing.T) {
	zero := time.Time{}

	start := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		b    Build
		want int64
	}{
		{"zero updatedAt", Build{CreatedAt: start, UpdatedAt: zero}, 0},
		{"zero createdAt", Build{CreatedAt: zero, UpdatedAt: start}, 0},
		{"normal", Build{CreatedAt: start, UpdatedAt: start.Add(12 * time.Second)}, 12},
		{"updatedAt before createdAt", Build{CreatedAt: start.Add(time.Second), UpdatedAt: start}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, buildDurationSeconds(c.b))
		})
	}
}

func TestLastN(t *testing.T) {
	mk := func(n int) []BuildLogEntry {
		out := make([]BuildLogEntry, n)
		for i := range out {
			out[i] = BuildLogEntry{Message: fmt.Sprintf("m-%d", i)}
		}

		return out
	}

	t.Run("returns all when len <= n", func(t *testing.T) {
		assert.Len(t, lastN(mk(3), 5), 3)
	})

	t.Run("trims to last n", func(t *testing.T) {
		got := lastN(mk(10), 3)
		require.Len(t, got, 3)
		assert.Equal(t, "m-7", got[0].Message)
		assert.Equal(t, "m-9", got[2].Message)
	})

	t.Run("returns input when n <= 0", func(t *testing.T) {
		in := mk(4)
		assert.Equal(t, in, lastN(in, 0))
		assert.Equal(t, in, lastN(in, -1))
	})
}

func TestParseBuildLogs(t *testing.T) {
	t.Run("valid JSONL passthrough", func(t *testing.T) {
		body := `{"asctime":"2026-06-09 10:00:00","levelname":"INFO","name":"image-builder","message":"start","build_id":"b-1"}
{"asctime":"2026-06-09 10:00:01","levelname":"DEBUG","name":"image-builder","message":"detail","build_id":"b-1"}
`
		entries, err := parseBuildLogs(strings.NewReader(body))
		require.NoError(t, err)
		require.Len(t, entries, 2)
		assert.Equal(t, "INFO", entries[0].Levelname)
		assert.Equal(t, "start", entries[0].Message)
		assert.Equal(t, "b-1", entries[0].BuildID)
		assert.NotEmpty(t, entries[0].Raw, "raw bytes preserved for passthrough")
	})

	t.Run("malformed lines are skipped", func(t *testing.T) {
		body := `{"levelname":"INFO","message":"ok"}
not-json-garbage
{"levelname":"ERROR","message":"bad"}
`
		entries, err := parseBuildLogs(strings.NewReader(body))
		require.NoError(t, err)
		require.Len(t, entries, 2)
		assert.Equal(t, "ok", entries[0].Message)
		assert.Equal(t, "bad", entries[1].Message)
	})

	t.Run("empty body returns empty slice", func(t *testing.T) {
		entries, err := parseBuildLogs(strings.NewReader(""))
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("blank lines are skipped", func(t *testing.T) {
		body := "\n\n{\"levelname\":\"INFO\",\"message\":\"x\"}\n\n"
		entries, err := parseBuildLogs(strings.NewReader(body))
		require.NoError(t, err)
		assert.Len(t, entries, 1)
	})
}

func TestBuildLogEntry_MarshalJSON_PassesThroughRaw(t *testing.T) {
	entry := BuildLogEntry{
		Asctime:   "later", // would disagree with Raw to prove passthrough wins
		Levelname: "INFO",
		Message:   "decoded",
		Raw:       json.RawMessage(`{"original":"untouched","levelname":"INFO"}`),
	}

	out, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.JSONEq(t, `{"original":"untouched","levelname":"INFO"}`, string(out))
}

func TestBuildLogEntry_MarshalJSON_FallsBackToFields(t *testing.T) {
	entry := BuildLogEntry{
		Asctime:   "2026-06-09",
		Levelname: "INFO",
		Message:   "decoded",
	}

	out, err := json.Marshal(entry)
	require.NoError(t, err)

	var got map[string]any

	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "INFO", got["levelname"])
	assert.Equal(t, "decoded", got["message"])
	assert.NotContains(t, got, "Raw")
	assert.NotContains(t, got, "raw")
}

func TestTriggerArtifactBuild_PostsEmptyBodyAndDecodes(t *testing.T) {
	installSkipAuth(t)

	var (
		gotPath   string
		gotMethod string
		gotBody   []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"buildIds":["b-1","b-2"]}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	resp, err := TriggerArtifactBuild("art-1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"b-1", "b-2"}, resp.BuildIDs)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v2/artifacts/art-1/builds/", gotPath)
	assert.JSONEq(t, `{}`, string(gotBody))
}

func TestTriggerArtifactBuild_PropagatesValidationError(t *testing.T) {
	installSkipAuth(t)

	const detail = `{"detail":[{"path":"id","message":"Artifact not found","code":"not_found"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(detail))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := TriggerArtifactBuild("art-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Artifact not found", "validation body must reach caller (0ae2527 regression)")
}

func TestGetArtifactBuild_Decodes(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/artifacts/art-1/builds/b-1", r.URL.Path)

		_, _ = w.Write([]byte(`{
			"id":"b-1",
			"name":"my build",
			"artifactId":"art-1",
			"status":"COMPLETED",
			"createdAt":"2026-06-09T10:00:00Z",
			"updatedAt":"2026-06-09T10:00:12Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build, err := GetArtifactBuild("art-1", "b-1")
	require.NoError(t, err)
	assert.Equal(t, "b-1", build.ID)
	assert.Equal(t, BuildStatusCompleted, build.Status)
	assert.Equal(t, int64(12), buildDurationSeconds(*build))
}

func TestGetArtifactBuild_NotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetArtifactBuild("art-1", "b-missing")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestListArtifactBuilds_PaginatesAndCaps(t *testing.T) {
	installSkipAuth(t)

	var (
		hits    int32
		nextURL string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := atomic.AddInt32(&hits, 1)

		assert.Equal(t, "/api/v2/artifacts/art-1/builds/", r.URL.Path)

		switch page {
		case 1:
			body := fmt.Sprintf(`{
				"data": [
					{"id":"b-1","artifactId":"art-1","status":"COMPLETED"},
					{"id":"b-2","artifactId":"art-1","status":"COMPLETED"}
				],
				"count": 2, "totalCount": 5,
				"next": "%s/api/v2/artifacts/art-1/builds/?limit=5&offset=2",
				"previous": null
			}`, nextURL)
			_, _ = w.Write([]byte(body))
		case 2:
			body := `{
				"data": [
					{"id":"b-3","artifactId":"art-1","status":"COMPLETED"},
					{"id":"b-4","artifactId":"art-1","status":"COMPLETED"},
					{"id":"b-5","artifactId":"art-1","status":"COMPLETED"}
				],
				"count": 3, "totalCount": 5,
				"next": "",
				"previous": null
			}`
			_, _ = w.Write([]byte(body))
		}
	}))

	defer srv.Close()

	nextURL = srv.URL

	installEndpoint(t, srv.URL)

	builds, err := ListArtifactBuilds("art-1", 5)
	require.NoError(t, err)
	require.Len(t, builds, 5)
	assert.Equal(t, "b-5", builds[4].ID)
}

func TestListArtifactBuilds_RespectsLimitCap(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"b-1","artifactId":"art-1","status":"COMPLETED"},
				{"id":"b-2","artifactId":"art-1","status":"COMPLETED"},
				{"id":"b-3","artifactId":"art-1","status":"COMPLETED"}
			],
			"count": 3, "totalCount": 100, "next": "", "previous": null
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	builds, err := ListArtifactBuilds("art-1", 2)
	require.NoError(t, err)
	assert.Len(t, builds, 2)
}

func TestListArtifactBuilds_InvalidLimit(t *testing.T) {
	_, err := ListArtifactBuilds("art-1", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit")
}

func TestGetArtifactBuildLogs_ParsesJSONL(t *testing.T) {
	installSkipAuth(t)

	body := `{"asctime":"2026-06-09 10:00:00","levelname":"INFO","name":"image-builder","message":"line-1","build_id":"b-1"}
{"asctime":"2026-06-09 10:00:01","levelname":"DEBUG","name":"image-builder","message":"line-2","build_id":"b-1"}
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/artifacts/art-1/builds/b-1/logs", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	entries, err := GetArtifactBuildLogs("art-1", "b-1")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "INFO", entries[0].Levelname)
	assert.Equal(t, "line-2", entries[1].Message)
}

func TestWaitForBuild_TerminalCompletedReturnsNil(t *testing.T) {
	installSkipAuth(t)

	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := atomic.AddInt32(&hits, 1)

		status := BuildStatusInProgress
		if page >= 2 {
			status = BuildStatusCompleted
		}

		fmt.Fprintf(w, `{
			"id":"b-1","artifactId":"art-1","status":"%s",
			"createdAt":"2026-06-09T10:00:00Z",
			"updatedAt":"2026-06-09T10:00:08Z"
		}`, status)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var ticks int

	build, err := WaitForBuild("art-1", "b-1", time.Millisecond, time.Second, func(*Build) {
		ticks++
	})
	require.NoError(t, err)
	assert.Equal(t, BuildStatusCompleted, build.Status)
	assert.GreaterOrEqual(t, ticks, 2)
}

func TestWaitForBuild_FailedReturnsError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"b-1","artifactId":"art-1","status":"FAILED",
			"createdAt":"2026-06-09T10:00:00Z",
			"updatedAt":"2026-06-09T10:00:08Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build, err := WaitForBuild("art-1", "b-1", time.Millisecond, time.Second, nil)
	require.Error(t, err)
	require.NotNil(t, build, "FAILED returns final Build alongside error")
	assert.Equal(t, BuildStatusFailed, build.Status)
	assert.Contains(t, err.Error(), "FAILED")
}

func TestWaitForBuild_Timeout(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"b-1","artifactId":"art-1","status":"PENDING",
			"createdAt":"2026-06-09T10:00:00Z",
			"updatedAt":"2026-06-09T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := WaitForBuild("art-1", "b-1", 5*time.Millisecond, 25*time.Millisecond, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestBuildSummaryFor_SuccessSkipsLogs(t *testing.T) {
	installSkipAuth(t)

	var logsHit bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			logsHit = true

			return
		}

		_, _ = w.Write([]byte(`{
			"id":"art-1","name":"a","status":"draft",
			"spec":{"containerGroups":[{"containers":[{"primary":true,"imageUri":"ecr/img:tag"}]}]},
			"createdAt":"2026-06-09T10:00:00Z","updatedAt":"2026-06-09T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build := &Build{
		ID:         "b-1",
		ArtifactID: "art-1",
		Status:     BuildStatusCompleted,
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 9, 10, 0, 12, 0, time.UTC),
	}

	summary, err := BuildSummaryFor(build, DefaultBuildLogTail)
	require.NoError(t, err)
	assert.Equal(t, "b-1", summary.BuildID)
	assert.Equal(t, BuildStatusCompleted, summary.Status)
	assert.Equal(t, int64(12), summary.DurationSeconds)
	assert.Equal(t, "ecr/img:tag", summary.ImageURI)
	assert.Empty(t, summary.LogTail)
	assert.False(t, logsHit, "logs endpoint must not be hit on success")
}

func TestBuildSummaryFor_FailureFetchesLogs(t *testing.T) {
	installSkipAuth(t)

	logBody := `{"levelname":"INFO","message":"start"}
{"levelname":"ERROR","message":"boom"}
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			_, _ = w.Write([]byte(logBody))

			return
		}

		_, _ = w.Write([]byte(`{
			"id":"art-1","name":"a","status":"draft",
			"spec":{"containerGroups":[{"containers":[{"primary":true,"imageUri":""}]}]},
			"createdAt":"2026-06-09T10:00:00Z","updatedAt":"2026-06-09T10:00:00Z"
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build := &Build{
		ID:         "b-1",
		ArtifactID: "art-1",
		Status:     BuildStatusFailed,
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 9, 10, 0, 9, 0, time.UTC),
	}

	summary, err := BuildSummaryFor(build, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(9), summary.DurationSeconds)
	assert.Empty(t, summary.ImageURI)
	require.Len(t, summary.LogTail, 1)
	assert.Equal(t, "boom", summary.LogTail[0].Message)
}

func TestBuildSummaryFor_NilBuild(t *testing.T) {
	_, err := BuildSummaryFor(nil, DefaultBuildLogTail)
	require.Error(t, err)
}

// TestBuildSummaryFor_TimeoutLeavesImageEmpty guards the non-terminal-on-
// timeout case: WaitForBuild returns the last-polled IN_PROGRESS build
// alongside a timeout error. BuildSummaryFor must NOT load the artifact
// (which would leak a stale imageUri from a prior successful build) and
// must NOT fetch logs (IN_PROGRESS is not an error status).
func TestBuildSummaryFor_TimeoutLeavesImageEmpty(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("BuildSummaryFor must not hit any endpoint when status is not COMPLETED and not an error (hit: %s)", r.URL.Path)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build := &Build{
		ID:         "b-1",
		ArtifactID: "art-1",
		Status:     BuildStatusInProgress,
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 9, 10, 10, 0, 0, time.UTC),
	}

	summary, err := BuildSummaryFor(build, DefaultBuildLogTail)
	require.NoError(t, err)
	assert.Equal(t, BuildStatusInProgress, summary.Status)
	assert.Equal(t, int64(600), summary.DurationSeconds)
	assert.Empty(t, summary.ImageURI, "non-terminal status must not leak a prior build's imageUri")
	assert.Empty(t, summary.LogTail, "non-error status must not fetch logs")
}

// TestBuildSummaryFor_FailureLogFetch502 reproduces the live CANCELLED-build
// smoke case: the build-service garbage-collected the log records and the
// /logs endpoint 502s. BuildSummaryFor must NOT propagate the log-fetch
// failure as an error; the summary stays valuable (duration + status) and
// LogTail is left nil so the caller can render the partial info.
func TestBuildSummaryFor_FailureLogFetch502(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			// Match the staging shape: 502 with a JSON detail body.
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"detail":"Failed to retrieve build logs"}`))

			return
		}

		// The skip-artifact-on-error branch must NOT hit this handler at
		// all for FAILED/CANCELLED builds; assert by failing if it does.
		t.Errorf("BuildSummaryFor must not fetch the parent artifact on error status (hit: %s)", r.URL.Path)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	build := &Build{
		ID:         "b-1",
		ArtifactID: "art-1",
		Status:     BuildStatusCancelled,
		CreatedAt:  time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 9, 10, 24, 8, 0, time.UTC),
	}

	summary, err := BuildSummaryFor(build, DefaultBuildLogTail)
	require.NoError(t, err, "log-fetch 502 must not fail the summary")
	assert.Equal(t, "b-1", summary.BuildID)
	assert.Equal(t, BuildStatusCancelled, summary.Status)
	assert.Equal(t, int64(1448), summary.DurationSeconds)
	assert.Empty(t, summary.ImageURI, "imageUri must not be set on error-status builds")
	assert.Nil(t, summary.LogTail, "LogTail stays nil so callers can detect 'no logs available'")
}
