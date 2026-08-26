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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchArtifactBuildLogs_FiltersOnExternalBuildID(t *testing.T) {
	installSkipAuth(t)

	var gotPath string

	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()

		fmt.Fprint(w, logsPage("", logEntryDocAt("2026-06-11 14:04:14.084208+00:00", "INFO", "step 1")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	entries, err := fetchArtifactBuildLogs("art-1", "bld-1", 10, "", "", "")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "step 1", entries[0].Message)

	assert.Equal(t, "/api/v2/otel/artifact/art-1/logs/", gotPath)
	// camelCase is load-bearing: the snake_case forms are rejected with 400.
	assert.Equal(t, []string{"external_build_id"}, gotQuery["searchKeys"])
	assert.Equal(t, []string{"bld-1"}, gotQuery["searchValues"])
}

func TestBuildLogTail_EmitsOnlyUnseenLinesAcrossPolls(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		switch calls {
		case 1:
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "b"),
				logEntryDocAt("2026-06-11 14:04:14.084208+00:00", "INFO", "a"),
			))
		default:
			// The overlap line b re-serves within the lag allowance; only c is new.
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:16.000001+00:00", "INFO", "c"),
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "b"),
			))
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	tail := NewBuildLogTail("art-1", "bld-1", func(e WorkloadLogEntry) {
		lines = append(lines, e.Message)
	}, nil)

	tail.Poll()
	tail.Poll()
	tail.flush(true) // drain the holdback buffer without another fetch

	assert.Equal(t, []string{"a", "b", "c"}, lines)
	assert.Equal(t, 2, calls)
}

func TestBuildLogTail_HoldbackReordersLateLines(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		switch calls {
		case 1:
			// The newer line arrives first...
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:16.000000+00:00", "INFO", "later"),
			))
		default:
			// ...its older sibling only in the NEXT poll, the ingestion
			// pattern that used to print "later" before "earlier".
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:16.000000+00:00", "INFO", "later"),
				logEntryDocAt("2026-06-11 14:04:15.000000+00:00", "INFO", "earlier"),
			))
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	tail := NewBuildLogTail("art-1", "bld-1", func(e WorkloadLogEntry) {
		lines = append(lines, e.Message)
	}, nil)

	tail.Poll()
	tail.Poll()
	tail.flush(true)

	// The holdback let the late older line slot in ahead.
	assert.Equal(t, []string{"earlier", "later"}, lines)
}

func TestBuildLogTail_EmptyFirstPollDoesNotPinAZeroCursor(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	var startTimes []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		startTimes = append(startTimes, r.URL.Query().Get("startTime"))

		switch calls {
		case 1:
			// The stream is empty right after the trigger.
			fmt.Fprint(w, logsPage(""))
		default:
			fmt.Fprint(w, logsPage("",
				logEntryDocAt("2026-06-11 14:04:15.000001+00:00", "INFO", "b"),
				logEntryDocAt("2026-06-11 14:04:14.084208+00:00", "INFO", "a"),
			))
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var lines []string

	tail := NewBuildLogTail("art-1", "bld-1", func(e WorkloadLogEntry) {
		lines = append(lines, e.Message)
	}, nil)

	tail.Poll() // empty: must NOT seed
	tail.Poll() // first real batch: window fetch, seeds the cursor
	tail.Poll() // since-based, trailing by the build tail's own lag
	tail.flush(true)

	assert.Equal(t, []string{"a", "b"}, lines)
	require.Len(t, startTimes, 3)
	// The empty poll and the first real batch are window fetches...
	assert.Empty(t, startTimes[0])
	assert.Empty(t, startTimes[1])
	// ...and the third trails the newest timestamp by the build lag, not the
	// runtime follower's default.
	newest, ok := parseLogTimestamp("2026-06-11 14:04:15.000001+00:00")
	require.True(t, ok)
	assert.Equal(t, newest.Add(-buildLogLagAllowance).UTC().Format(time.RFC3339Nano), startTimes[2])
}

func TestBuildLogTail_DisablesAfterSustainedFailuresWithoutErroring(t *testing.T) {
	installSkipAuth(t)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	var warns []string

	tail := NewBuildLogTail("art-1", "bld-1", func(WorkloadLogEntry) {
		t.Fatal("no line should be emitted from a failing stream")
	}, func(w string) { warns = append(warns, w) })

	// One more poll than the transient budget: the last one must be a no-op.
	for range maxTransientPollErrors + 2 {
		tail.Poll()
	}

	assert.True(t, tail.disabled)
	assert.Equal(t, maxTransientPollErrors+1, calls, "a disabled tail must stop fetching")
	// Exactly one user-visible notice that streaming gave up, at the end.
	require.NotEmpty(t, warns)
	assert.Contains(t, warns[len(warns)-1], "the build itself is unaffected")
}
