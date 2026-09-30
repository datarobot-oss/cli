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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server-side half of a filter travels as the query parameters the route
// accepts, measured on staging: camelCase, one search term on the message,
// and a window in RFC 3339 with a Z suffix, which is the one spelling it
// takes (RAPTOR-18069).
func TestGetWorkloadLogs_SendsTheServerSideFilter(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "error", q.Get("level"))
		assert.Equal(t, "message", q.Get("searchKeys"))
		assert.Equal(t, "connection refused", q.Get("searchValues"), "only the first term goes to the server")
		assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", q.Get("traceId"))
		assert.Equal(t, "00f067aa0ba902b7", q.Get("spanId"))
		assert.Equal(t, "2026-06-11T12:00:00Z", q.Get("startTime"), "UTC with a Z, never an offset")
		assert.Equal(t, "2026-06-11T14:00:00Z", q.Get("endTime"))

		// Nothing the route would refuse.
		for key := range q {
			assert.Contains(t, []string{"limit", "level", "searchKeys", "searchValues", "traceId", "spanId", "startTime", "endTime"}, key)
		}

		fmt.Fprint(w, logsPage("", logEntryDoc("ERROR", "connection refused by upstream")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	berlin := time.FixedZone("CEST", 2*60*60)

	entries, err := GetWorkloadLogs("wl-1", 25, LogFilter{
		Level:   "error",
		Grep:    []string{"connection refused", "upstream"},
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Since:   time.Date(2026, 6, 11, 14, 0, 0, 0, berlin),
		Until:   time.Date(2026, 6, 11, 16, 0, 0, 0, berlin),
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// A filter with nothing to say sends nothing: the route refuses parameters
// it does not know, and an empty one is not the same as an absent one.
func TestGetWorkloadLogs_EmptyFilterSendsOnlyTheLimit(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, []string{"limit"}, keysOf(r.URL.Query()))
		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 25, LogFilter{})
	require.NoError(t, err)
}

func keysOf(values map[string][]string) []string {
	keys := make([]string, 0, len(values))

	for k := range values {
		keys = append(keys, k)
	}

	return keys
}

// The client-side half: every search term must match, no exclusion may,
// and case is ignored throughout. The server has no exclusion parameter and
// its reading of a second term was not one worth depending on.
func TestLogFilter_Keep(t *testing.T) {
	lines := []WorkloadLogEntry{
		{Message: "GET /healthz 200"},
		{Message: "Connection refused by upstream"},
		{Message: "connection REFUSED: retrying"},
		{Message: "upstream ok"},
	}

	messages := func(entries []WorkloadLogEntry) []string {
		out := make([]string, 0, len(entries))

		for _, e := range entries {
			out = append(out, e.Message)
		}

		return out
	}

	for _, tc := range []struct {
		name   string
		filter LogFilter
		want   []string
	}{
		{"nothing client-side keeps everything", LogFilter{Level: "info", TraceID: "t"}, messages(lines)},
		{
			"one term, any case",
			LogFilter{Grep: []string{"REFUSED"}},
			[]string{"Connection refused by upstream", "connection REFUSED: retrying"},
		},
		{
			"two terms, both required",
			LogFilter{Grep: []string{"refused", "upstream"}},
			[]string{"Connection refused by upstream"},
		},
		{
			"an exclusion",
			LogFilter{Exclude: []string{"healthz"}},
			[]string{"Connection refused by upstream", "connection REFUSED: retrying", "upstream ok"},
		},
		{
			"search and exclusion together",
			LogFilter{Grep: []string{"upstream"}, Exclude: []string{"refused"}},
			[]string{"upstream ok"},
		},
		{"everything excluded is an empty list, not nil", LogFilter{Exclude: []string{"e"}}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, messages(tc.filter.keep(lines)))
		})
	}
}

// The second search term is checked here even though the first went to the
// server, and it is what drops a line the server let through.
func TestGetWorkloadLogs_AppliesTheClientSideFilterAfterTheFetch(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, logsPage("",
			logEntryDoc("INFO", "refused: healthz probe"),
			logEntryDoc("INFO", "refused by upstream"),
			logEntryDoc("INFO", "refused, unrelated"),
		))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	entries, err := GetWorkloadLogs("wl-1", 25, LogFilter{Grep: []string{"refused", "upstream"}, Exclude: []string{"healthz"}})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "refused by upstream", entries[0].Message)
}

func TestParseLogTime(t *testing.T) {
	now := time.Date(2026, 6, 11, 14, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		value string
		want  time.Time
	}{
		{"2026-06-11T12:30:00Z", time.Date(2026, 6, 11, 12, 30, 0, 0, time.UTC)},
		{"2026-06-11T12:30:00.5+02:00", time.Date(2026, 6, 11, 12, 30, 0, 500_000_000, time.FixedZone("", 2*60*60))},
		{"2026-06-11T12:30:00", time.Date(2026, 6, 11, 12, 30, 0, 0, time.UTC)},
		{"2026-06-11 12:30:00", time.Date(2026, 6, 11, 12, 30, 0, 0, time.UTC)},
		{"2026-06-11", time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)},
		{"15m", now.Add(-15 * time.Minute)},
		{"2h30m", now.Add(-150 * time.Minute)},
		{"1d", now.Add(-24 * time.Hour)},
		{"1.5d", now.Add(-36 * time.Hour)},
		{"2w", now.Add(-14 * 24 * time.Hour)},
		{" 1h ", now.Add(-time.Hour)},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, err := ParseLogTime(tc.value, now)
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(got), "want %s, got %s", tc.want, got)
		})
	}

	for _, value := range []string{"", "yesterday", "2026-13-01", "-1h", "0d", "1y", "15"} {
		t.Run("rejects "+value, func(t *testing.T) {
			_, err := ParseLogTime(value, now)
			require.Error(t, err)
		})
	}
}

// Following with a filter: the seed carries the window's start and the
// server-side terms, later polls carry the cursor instead of Since, and the
// client-side half applies to what is printed without disturbing the cursor.
func TestFollowWorkloadLogs_FilterSeedsSinceThenFollowsTheCursor(t *testing.T) {
	installSkipAuth(t)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		q := r.URL.Query()
		assert.Equal(t, "message", q.Get("searchKeys"))
		assert.Equal(t, "refused", q.Get("searchValues"))

		switch calls {
		case 1:
			// The seed is bounded by --since, not by a cursor.
			assert.Equal(t, "2026-06-11T14:00:00Z", q.Get("startTime"))
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "refused: healthz"),
				logEntryDocAt("2026-06-11 14:04:14.084208+00:00", "INFO", "refused by upstream"),
			))
		default:
			// Later polls start from the cursor: the newest seed timestamp
			// minus the lag allowance, even though that line was excluded
			// from the output, so nothing after it is missed.
			newest, ok := parseLogTimestamp("2026-06-11 14:04:15.000001+00:00")
			assert.True(t, ok)
			assert.Equal(t, newest.Add(-followLagAllowance).UTC().Format(time.RFC3339Nano), q.Get("startTime"))
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:16.000001+00:00", "INFO", "refused again"),
			))

			cancel()
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	filter := LogFilter{
		Grep:    []string{"refused"},
		Exclude: []string{"healthz"},
		Since:   time.Date(2026, 6, 11, 14, 0, 0, 0, time.UTC),
	}

	err := FollowWorkloadLogs(ctx, "wl-1", 5, filter, time.Millisecond,
		func(e WorkloadLogEntry) error {
			lines = append(lines, e.Message)

			return nil
		}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"refused by upstream", "refused again"}, lines)
	assert.GreaterOrEqual(t, calls, 2)
}

// A follow has no end, so an end time is refused before the first request.
func TestFollowWorkloadLogs_RefusesAnEndTime(t *testing.T) {
	err := FollowWorkloadLogs(context.Background(), "wl-1", 5, LogFilter{Until: time.Now()}, time.Second,
		func(WorkloadLogEntry) error { return nil }, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end time")
}
