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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logEntryDoc(level, message string) string {
	return fmt.Sprintf(`{"timestamp": "2026-06-11 14:04:14.084208+00:00", "level": %q, "message": %q}`, level, message)
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
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		fmt.Fprint(w, logsPage("", logEntryDoc("INFO", "x")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkloadLogs("wl-1", 250, "")
	require.NoError(t, err)
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

func TestFilterUnseenLogs(t *testing.T) {
	seen := make(map[string]struct{})

	first := []WorkloadLogEntry{
		{Timestamp: "t1", Level: "INFO", Message: "a"},
		{Timestamp: "t2", Level: "INFO", Message: "b"},
	}

	// First poll: everything is new, order preserved.
	fresh := FilterUnseenLogs(first, seen)
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

	fresh = FilterUnseenLogs(second, seen)
	require.Len(t, fresh, 1)
	assert.Equal(t, "c", fresh[0].Message)
}

func TestFilterUnseenLogs_SameTimestampDistinctMessages(t *testing.T) {
	seen := make(map[string]struct{})

	// Two lines in the same microsecond must both be treated as distinct.
	entries := []WorkloadLogEntry{
		{Timestamp: "t1", Level: "INFO", Message: "first"},
		{Timestamp: "t1", Level: "INFO", Message: "second"},
	}

	fresh := FilterUnseenLogs(entries, seen)
	assert.Len(t, fresh, 2)
}
