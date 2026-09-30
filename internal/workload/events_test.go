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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventsFixture is the platform's own answer, captured on staging: the trail
// of a workload that had two settings changes roll out and one fail. The
// entries are not in time order, which is the point of the sort.
func eventsFixture(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "workload_events.json"))
	require.NoError(t, err)

	return string(data)
}

func serveEvents(t *testing.T, body string) {
	t.Helper()

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/wl-1/events/", r.URL.Path)
		assert.Equal(t, []string{"limit"}, keysOf(r.URL.Query()), "the route takes nothing else")

		fmt.Fprint(w, body)
	}))
}

func TestListWorkloadEvents_ReadsTheFixtureOldestFirst(t *testing.T) {
	serveEvents(t, eventsFixture(t))

	events, err := ListWorkloadEvents("wl-1", 100, EventFilter{})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Sorted by time, not left in the order the server used: the errored
	// entry was recorded before the completion it sits after on the wire.
	assert.Equal(t, "Replacement Completed", events[0].EventType)
	assert.Equal(t, "Replacement Errored", events[1].EventType)
	assert.Equal(t, "Replacement Completed", events[2].EventType)
	assert.True(t, events[0].Timestamp.Before(events[1].Timestamp))
	assert.True(t, events[1].Timestamp.Before(events[2].Timestamp))

	assert.Equal(t, "683e3b2eb7aefb434d49763b", events[0].ActorID)
	assert.Equal(t, "6abbbaea720810ff290d71eb", events[0].WorkloadID)
	assert.Contains(t, events[1].Message(), "ResourceBundleInferenceError")

	// The details survive as the platform wrote them.
	var details map[string]any

	require.NoError(t, json.Unmarshal(events[0].Details, &details))
	assert.Equal(t, []any{"6abd4feb9a6fa4c38620c751"}, details["candidateProtonIds"])
}

// The most recent limit events are kept, still oldest first.
func TestListWorkloadEvents_LimitKeepsTheMostRecent(t *testing.T) {
	serveEvents(t, eventsFixture(t))

	events, err := ListWorkloadEvents("wl-1", 2, EventFilter{})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "Replacement Errored", events[0].EventType)
	assert.Equal(t, "Replacement Completed", events[1].EventType)
}

func TestListWorkloadEvents_Filters(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339Nano, s)
		require.NoError(t, err)

		return ts
	}

	for _, tc := range []struct {
		name   string
		filter EventFilter
		want   []string
	}{
		{"by type, any case, as a substring", EventFilter{Types: []string{"errored"}}, []string{"Replacement Errored"}},
		{
			"several types, any of them",
			EventFilter{Types: []string{"errored", "COMPLETED"}},
			[]string{"Replacement Completed", "Replacement Errored", "Replacement Completed"},
		},
		{"a type nothing matches", EventFilter{Types: []string{"deleted"}}, []string{}},
		{"since", EventFilter{Since: at("2026-09-30T18:10:00Z")}, []string{"Replacement Errored", "Replacement Completed"}},
		{"until", EventFilter{Until: at("2026-09-30T18:10:00Z")}, []string{"Replacement Completed"}},
		{"a generation named in the details", EventFilter{ProtonID: "6abd4e2d9a6fa4c38620c74f"}, []string{"Replacement Completed"}},
		{"a generation nothing names", EventFilter{ProtonID: "nope"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveEvents(t, eventsFixture(t))

			events, err := ListWorkloadEvents("wl-1", 100, tc.filter)
			require.NoError(t, err)

			got := make([]string, 0, len(events))
			for _, e := range events {
				got = append(got, e.EventType)
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListWorkloadEvents_FollowsNextLinks(t *testing.T) {
	installSkipAuth(t)

	var srv *httptest.Server

	calls := 0

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		switch calls {
		case 1:
			assert.Empty(t, r.URL.Query().Get("offset"))
			fmt.Fprintf(w, `{"totalCount":2,"count":1,"next":%q,"previous":null,"data":[
				{"id":"e1","workloadId":"wl-1","timestamp":"2026-09-30T18:00:00Z","eventType":"Created","actorId":"u1","details":null}]}`,
				srv.URL+"/api/v2/workloads/wl-1/events/?limit=1&offset=1")
		default:
			assert.Equal(t, "1", r.URL.Query().Get("offset"), "the next link is followed as given")
			fmt.Fprint(w, `{"totalCount":2,"count":1,"next":null,"previous":null,"data":[
				{"id":"e2","workloadId":"wl-1","timestamp":"2026-09-30T18:01:00Z","eventType":"Started","actorId":"u1","details":{"message":"up"}}]}`)
		}
	}))

	t.Cleanup(srv.Close)
	installEndpoint(t, srv.URL)

	events, err := ListWorkloadEvents("wl-1", 100, EventFilter{})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, events, 2)
	assert.Equal(t, "Created", events[0].EventType)
	assert.Equal(t, "up", events[1].Message())
}

func TestListWorkloadEvents_RejectsANonPositiveLimit(t *testing.T) {
	_, err := ListWorkloadEvents("wl-1", 0, EventFilter{})
	require.Error(t, err)
}

// The table carries the platform's sentence when there is one and the raw
// details otherwise; JSON is one envelope with [] for an empty trail.
func TestRenderWorkloadEvents(t *testing.T) {
	events := []WorkloadEvent{
		{
			Timestamp: time.Date(2026, 9, 30, 18, 7, 59, 0, time.UTC), EventType: "Replacement Completed", ActorID: "u1",
			Details: json.RawMessage(`{"message":"Maintenance completed.","replacementId":"r1"}`),
		},
		{Timestamp: time.Date(2026, 9, 30, 18, 8, 0, 0, time.UTC), EventType: "Scaled", Details: json.RawMessage(`{"replicas":3}`)},
	}

	var text bytes.Buffer

	require.NoError(t, RenderWorkloadEventsTo(&text, outputformat.OutputFormatText, events))
	assert.Contains(t, text.String(), "Replacement Completed")
	assert.Contains(t, text.String(), "Maintenance completed.")
	assert.NotContains(t, text.String(), "replacementId", "a sentence stands in for the details")
	assert.Contains(t, text.String(), `{"replicas":3}`, "details with no sentence are shown as they are")
	assert.Contains(t, text.String(), "u1")

	// On a terminal the table is bounded and a long message wraps inside
	// its column rather than spilling past the right edge.
	events[0].Details = json.RawMessage(`{"message":"` + strings.Repeat("word ", 60) + `"}`)

	for _, line := range strings.Split(eventsTable(events, 100), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 100, "line wider than the terminal: %q", line)
	}

	var empty bytes.Buffer

	require.NoError(t, RenderWorkloadEventsTo(&empty, outputformat.OutputFormatText, nil))
	assert.Equal(t, "No events found.\n", empty.String())

	var out bytes.Buffer

	require.NoError(t, RenderWorkloadEventsTo(&out, outputformat.OutputFormatJSON, nil))

	var envelope map[string]any

	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope))
	assert.Equal(t, []any{}, envelope["events"], "an empty trail is [], never null")
}
