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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logEntryDocAt(timestamp, level, message string) string {
	return fmt.Sprintf(`{"timestamp": %q, "level": %q, "message": %q}`, timestamp, level, message)
}

func logEntryDoc(level, message string) string {
	return logEntryDocAt("2026-06-11 14:04:14.084208+00:00", level, message)
}

func logsPage(next string, entries ...string) string {
	nextJSON := "null"
	if next != "" {
		nextJSON = fmt.Sprintf("%q", next)
	}

	return fmt.Sprintf(
		`{"data": [%s], "count": %d, "next": %s, "previous": null}`,
		strings.Join(entries, ","), len(entries), nextJSON,
	)
}

func TestGetWorkloadLogs_SinglePageReversedToChronological(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		// The OTEL logs endpoint sits under the public gateway, with a
		// trailing slash.
		assert.Equal(t, "/api/v2/otel/workload/wl-1/logs/", r.URL.Path)
		assert.Equal(t, "25", r.URL.Query().Get("limit"))

		// Server returns newest first.
		fmt.Fprint(w, logsPage("",
			logEntryDoc("INFO", "newest"),
			logEntryDoc("INFO", "oldest"),
		))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 25, "")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	// Reversed for display: oldest first, newest last.
	assert.Equal(t, "oldest", entries[0].Message)
	assert.Equal(t, "newest", entries[1].Message)
	assert.Equal(t, "INFO", entries[1].Level)
}

func TestGetWorkloadLogs_LevelLowercased(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "error", r.URL.Query().Get("level"))
		fmt.Fprint(w, logsPage("", logEntryDoc("ERROR", "boom")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 25, "ERROR")
	require.NoError(t, err)
}

func TestGetWorkloadLogs_OmitsLevelWhenEmpty(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasLevel := r.URL.Query()["level"]
		assert.False(t, hasLevel, "level must be omitted when not requested")

		_, hasStart := r.URL.Query()["startTime"]
		assert.False(t, hasStart, "startTime must be omitted outside follow mode")

		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 25, "")
	require.NoError(t, err)
}

func TestGetWorkloadLogs_FollowsNextAndTruncatesToLimit(t *testing.T) {
	installSkipAuth(t)

	var srvURL string

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		switch calls {
		case 1:
			next := srvURL + "/api/v2/otel/workload/wl-1/logs/?offset=2&limit=3"
			fmt.Fprint(w, logsPage(next, logEntryDoc("INFO", "n4"), logEntryDoc("INFO", "n3")))
		default:
			fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "n2"), logEntryDoc("INFO", "n1")))
		}
	}))

	defer srv.Close()

	srvURL = srv.URL

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 3, "")
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, entries, 3)
	// Newest-first [n4,n3,n2] truncated then reversed -> [n2,n3,n4].
	assert.Equal(t, "n2", entries[0].Message)
	assert.Equal(t, "n4", entries[2].Message)
}

func TestGetWorkloadLogs_ClampsPageSizeToServerMax(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The logs endpoint's offset/limit validator accepts 1..1000.
		assert.Equal(t, "1000", r.URL.Query().Get("limit"))
		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 2500, "")
	require.NoError(t, err)
}

func TestGetWorkloadLogs_StopsOnEmptyPageWithNext(t *testing.T) {
	installSkipAuth(t)

	var srvURL string

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		// A server stuck on an empty page that still advertises a next link
		// must not spin the fetch forever.
		next := srvURL + "/api/v2/otel/workload/wl-1/logs/?offset=0&limit=5"
		fmt.Fprint(w, logsPage(next))
	}))

	defer srv.Close()

	srvURL = srv.URL

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 5, "")
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Equal(t, 1, calls)
}

func TestGetWorkloadLogs_DropsCrossPageDuplicates(t *testing.T) {
	installSkipAuth(t)

	var srvURL string

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		switch calls {
		case 1:
			next := srvURL + "/api/v2/otel/workload/wl-1/logs/?offset=2&limit=10"
			fmt.Fprint(w, logsPage(next,
				logEntryDocAt("t4", "INFO", "n4"),
				logEntryDocAt("t3", "INFO", "n3"),
			))
		default:
			// A line arriving between the two page fetches shifts the offsets,
			// so the second page re-serves n3; the boundary dedup drops it.
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("t3", "INFO", "n3"),
				logEntryDocAt("t2", "INFO", "n2"),
			))
		}
	}))

	defer srv.Close()

	srvURL = srv.URL

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 10, "")
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "n2", entries[0].Message)
	assert.Equal(t, "n3", entries[1].Message)
	assert.Equal(t, "n4", entries[2].Message)
}

func TestGetWorkloadLogs_RejectsOffHostNext(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A buggy or compromised server response pointing next at a
		// different host would otherwise leak the bearer token on the
		// next request; AssertNextOnSameHost must refuse it.
		fmt.Fprint(w, logsPage(
			"http://evil.example.com/api/v2/otel/workload/wl-1/logs/?offset=2",
			logEntryDoc("INFO", "x"),
		))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match API base host")
}

func TestGetWorkloadLogs_RejectsMalformedNext(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A malformed next URL must surface as an error rather than being
		// silently rebased against the API host. The embedded newline makes
		// url.Parse fail with "invalid control character in URL".
		fmt.Fprint(w, logsPage("http://example.com\n/oops", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse Next URL")
}

func TestGetWorkloadLogs_KeepsSamePageDuplicates(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Two genuinely identical lines served together are real output, not
		// a pagination artifact, and must both be shown.
		fmt.Fprint(w, logsPage("",
			logEntryDocAt("t1", "INFO", "again"),
			logEntryDocAt("t1", "INFO", "again"),
		))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 10, "")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestGetWorkloadLogs_RejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		_, err := GetWorkloadLogs("wl-1", limit, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	}
}

func TestGetWorkloadLogs_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/otel/workload/wl-1%3Fx=1/logs/", r.URL.EscapedPath())
		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1?x=1", 25, "")
	require.NoError(t, err)
}

func TestGetWorkloadLogs_404PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("missing", 25, "")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestParseLogLevel(t *testing.T) {
	// Empty stays empty: the server then applies its debug default.
	parsed, err := ParseLogLevel("")
	require.NoError(t, err)
	assert.Empty(t, parsed)

	for _, valid := range []string{"debug", "INFO", " warning ", "warn", "Error", "critical"} {
		parsed, err := ParseLogLevel(valid)
		require.NoErrorf(t, err, "level %q", valid)
		assert.Equal(t, strings.ToLower(strings.TrimSpace(valid)), parsed)
	}

	_, err = ParseLogLevel("eror")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid log level "eror"`)
	assert.Contains(t, err.Error(), "debug, info, warn, warning, error, critical")
}

func TestParseLogTimestamp(t *testing.T) {
	// The live gateway's shape: space-separated, microseconds, numeric offset.
	parsed, ok := parseLogTimestamp("2026-06-11 14:04:14.084208+00:00")
	require.True(t, ok)
	assert.Equal(t, 84208000, parsed.Nanosecond())

	// Without fractional seconds, and in RFC3339, both still parse.
	_, ok = parseLogTimestamp("2026-06-11 14:04:14+00:00")
	assert.True(t, ok)

	_, ok = parseLogTimestamp("2026-06-11T14:04:14.084208Z")
	assert.True(t, ok)

	// Unrecognized shapes report false so the follow cursor stays off.
	_, ok = parseLogTimestamp("t1")
	assert.False(t, ok)
}

func TestLogDedup_FiltersAcrossCalls(t *testing.T) {
	dedup := newLogDedup(100)

	first := []WorkloadLogEntry{
		{Timestamp: "t1", Level: "INFO", Message: "a"},
		{Timestamp: "t2", Level: "INFO", Message: "b"},
	}

	// First poll: everything is new, order preserved.
	fresh := dedup.filterUnseen(first)
	require.Len(t, fresh, 2)
	assert.Equal(t, "a", fresh[0].Message)
	assert.Equal(t, "b", fresh[1].Message)

	// Second poll overlaps the first two and adds one; only the new line
	// comes back.
	second := []WorkloadLogEntry{
		{Timestamp: "t1", Level: "INFO", Message: "a"},
		{Timestamp: "t2", Level: "INFO", Message: "b"},
		{Timestamp: "t3", Level: "INFO", Message: "c"},
	}

	fresh = dedup.filterUnseen(second)
	require.Len(t, fresh, 1)
	assert.Equal(t, "c", fresh[0].Message)
}

func TestLogDedup_SameTimestampDistinctMessages(t *testing.T) {
	dedup := newLogDedup(100)

	// Two lines in the same microsecond must both be treated as distinct.
	entries := []WorkloadLogEntry{
		{Timestamp: "t1", Level: "INFO", Message: "first"},
		{Timestamp: "t1", Level: "INFO", Message: "second"},
	}

	fresh := dedup.filterUnseen(entries)
	assert.Len(t, fresh, 2)
}

func TestLogDedup_RotationKeepsRecentMemory(t *testing.T) {
	entry := func(n int) WorkloadLogEntry {
		return WorkloadLogEntry{Timestamp: fmt.Sprintf("t%d", n), Level: "INFO", Message: "m"}
	}

	dedup := newLogDedup(2)

	// Fill past the generation cap: rotation must not forget lines that
	// were just emitted.
	for n := range 4 {
		fresh := dedup.filterUnseen([]WorkloadLogEntry{entry(n)})
		assert.Len(t, fresh, 1)
	}

	// All four are still remembered across the rotation (cap 2 keeps the
	// current and previous generations).
	for n := range 4 {
		assert.Emptyf(t, dedup.filterUnseen([]WorkloadLogEntry{entry(n)}), "entry %d re-emitted", n)
	}
}

func TestFollowWorkloadLogs_StreamsWithTimeCursor(t *testing.T) {
	installSkipAuth(t)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		switch calls {
		case 1:
			// Seed poll: plain newest-limit window, no time filter.
			assert.Empty(t, r.URL.Query().Get("startTime"))
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "b"),
				logEntryDocAt("2026-06-11 14:04:14.084208+00:00", "INFO", "a"),
			))
		default:
			// Follow polls filter server-side by the lag-adjusted cursor:
			// the newest seed timestamp minus followLagAllowance, in RFC3339.
			newest, ok := parseLogTimestamp("2026-06-11 14:04:15.000001+00:00")
			assert.True(t, ok)
			assert.Equal(t,
				newest.Add(-followLagAllowance).UTC().Format(time.RFC3339Nano),
				r.URL.Query().Get("startTime"))

			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:16.000001+00:00", "INFO", "c"),
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "b"),
			))

			cancel()
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	err := FollowWorkloadLogs(ctx, "wl-1", 5, "", time.Millisecond,
		func(e WorkloadLogEntry) error {
			lines = append(lines, e.Message)

			return nil
		}, nil)
	require.NoError(t, err)
	// Chronological, with the overlap line b deduplicated on the second poll.
	assert.Equal(t, []string{"a", "b", "c"}, lines)
	assert.GreaterOrEqual(t, calls, 2)
}

func TestFollowWorkloadLogs_CancelledContextEndsCleanly(t *testing.T) {
	installSkipAuth(t)

	ctx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cancel()

		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	// Ctrl-C cancels the CLI's root context; the follow must end with nil
	// (a stopped tail is a normal exit, not a failure) instead of looping.
	err := FollowWorkloadLogs(ctx, "wl-1", 5, "", time.Minute, func(WorkloadLogEntry) error { return nil }, nil)
	require.NoError(t, err)
}

func TestFollowWorkloadLogs_TerminalErrorStops(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := FollowWorkloadLogs(context.Background(), "missing", 5, "", time.Millisecond,
		func(WorkloadLogEntry) error { return nil }, nil)
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Equal(t, 1, calls)
}

func TestFollowWorkloadLogs_GivesUpAfterSustainedTransientErrors(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		w.WriteHeader(http.StatusBadGateway)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var warnings []string

	err := FollowWorkloadLogs(context.Background(), "wl-1", 5, "", time.Millisecond,
		func(WorkloadLogEntry) error { return nil },
		func(msg string) { warnings = append(warnings, msg) })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consecutive transient errors")
	// One initial poll plus retries until the cap is exceeded, with a
	// warning per retried failure.
	assert.Equal(t, maxTransientPollErrors+1, calls)
	assert.Len(t, warnings, maxTransientPollErrors)
}

func TestFollowWorkloadLogs_RecoversFromTransientErrors(t *testing.T) {
	installSkipAuth(t)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		// Two transient failures, then a real response. emit() resets the
		// transient counter on the success, so the follow keeps streaming
		// instead of accumulating toward the give-up cap.
		if calls <= 2 {
			w.WriteHeader(http.StatusBadGateway)

			return
		}

		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "ok")))

		cancel()
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	var warnings []string

	err := FollowWorkloadLogs(ctx, "wl-1", 5, "", time.Millisecond,
		func(e WorkloadLogEntry) error {
			lines = append(lines, e.Message)

			return nil
		},
		func(msg string) { warnings = append(warnings, msg) })
	require.NoError(t, err)
	assert.Equal(t, []string{"ok"}, lines)
	// Each transient failure produced one retry warning before the third
	// call succeeded.
	assert.Len(t, warnings, 2)
	assert.Equal(t, 3, calls)
}

func TestFollowWorkloadLogs_FallsBackWhenTimeFilterRejected(t *testing.T) {
	// Both 400 and 422 must classify as filter rejection; exercising both
	// catches a regression that drops one from isFilterRejectedError.
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			installSkipAuth(t)

			ctx, cancel := context.WithCancel(context.Background())

			defer cancel()

			calls := 0

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++

				switch {
				case calls == 1:
					fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "a")))
				case r.URL.Query().Get("startTime") != "":
					// An older server rejects the filter outright.
					w.WriteHeader(status)
				default:
					// The follow must retry without the filter and keep streaming.
					fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "b"), logEntryDoc("INFO", "a")))
					cancel()
				}
			}))

			defer srv.Close()

			installEndpoint(t, srv.URL)

			var lines []string

			var warnings []string

			err := FollowWorkloadLogs(ctx, "wl-1", 5, "", time.Millisecond,
				func(e WorkloadLogEntry) error {
					lines = append(lines, e.Message)

					return nil
				},
				func(msg string) { warnings = append(warnings, msg) })
			require.NoError(t, err)
			assert.Equal(t, []string{"a", "b"}, lines)
			require.NotEmpty(t, warnings)
			assert.Contains(t, warnings[0], "rejected the time filter")
			assert.Equal(t, 3, calls)
		})
	}
}

func TestFollowWorkloadLogs_WarnsOnWindowGap(t *testing.T) {
	installSkipAuth(t)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		// Unparseable timestamps keep the follow in window mode (no
		// startTime filter is ever sent).
		assert.Empty(t, r.URL.Query().Get("startTime"))

		if calls == 1 {
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("t2", "INFO", "b"),
				logEntryDocAt("t1", "INFO", "a"),
			))

			return
		}

		// A full window with zero overlap means lines were missed between
		// the polls.
		fmt.Fprint(w, logsPage("",
			logEntryDocAt("t9", "INFO", "z"),
			logEntryDocAt("t8", "INFO", "y"),
		))

		cancel()
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var warnings []string

	err := FollowWorkloadLogs(ctx, "wl-1", 2, "", time.Millisecond,
		func(WorkloadLogEntry) error { return nil },
		func(msg string) { warnings = append(warnings, msg) })
	require.NoError(t, err)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "possible gap")
}

func TestFollowWorkloadLogs_OnLineErrorStops(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	wantErr := errors.New("broken pipe")

	err := FollowWorkloadLogs(context.Background(), "wl-1", 5, "", time.Minute,
		func(WorkloadLogEntry) error { return wantErr }, nil)
	require.ErrorIs(t, err, wantErr)
}

func TestFollowWorkloadLogs_RejectsBadArguments(t *testing.T) {
	err := FollowWorkloadLogs(context.Background(), "wl-1", 0, "", time.Second, func(WorkloadLogEntry) error { return nil }, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit")

	err = FollowWorkloadLogs(context.Background(), "wl-1", 5, "", 0, func(WorkloadLogEntry) error { return nil }, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interval")

	// A nil onLine would panic in emit(); reject it up front instead.
	err = FollowWorkloadLogs(context.Background(), "wl-1", 5, "", time.Second, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "onLine callback is required")
}
