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

package logs

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The diagnose hint goes with an empty answer on the plain path only: not
// under JSON, where stderr stays clear, and not under --level, where an
// empty answer is the usual one for a healthy workload.
func TestCmd_EmptyAnswerPointsAtDiagnose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data": [], "count": 0, "next": null, "previous": null}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	run := func(args ...string) string {
		cmd := Cmd()
		cmd.PreRunE = nil
		cmd.SetArgs(append([]string{"68b0c1d2e3f4a5b6c7d8e9f0"}, args...))

		var stderr bytes.Buffer

		cmd.SetErr(&stderr)
		require.NoError(t, cmd.Execute())

		return stderr.String()
	}

	assert.Contains(t, run(), "dr workload diagnose 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, run("--level", "error"), "diagnose", "no error lines is the usual answer under a level")
	assert.Empty(t, run("--output-format", "json"), "JSON keeps stderr clean")
}

func TestCmd_ArgIsOptional(t *testing.T) {
	// Args is called directly: with the id optional, cmd.Execute() would run
	// PreRunE and start a real authentication flow.
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func TestCmd_RejectsNonPositiveLimit(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--limit", "0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a positive integer")
}

func TestCmd_ParsesFollowFlag(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil

	// -f is the shorthand for --follow.
	require.NoError(t, cmd.ParseFlags([]string{"-f", "--poll-interval", "500ms"}))

	follow, _ := cmd.Flags().GetBool("follow")
	interval, _ := cmd.Flags().GetDuration("poll-interval")

	assert.True(t, follow)
	assert.Equal(t, "500ms", interval.String())
}

func TestCmd_ListsPollInterval(t *testing.T) {
	assert.False(t, Cmd().Flag("poll-interval").Hidden)
}

func TestCmd_RejectsInvalidLevel(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--level", "eror"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid log level "eror"`)
}

func TestCmd_RejectsNonPositivePollInterval(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--follow", "--poll-interval", "0s"})

	// Rejected at flag-parse time by pollflags.PositiveDuration, before any
	// request is made.
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a positive duration")
}

// The filter flags are checked against each other before anything reaches
// the network, and the error names the flag at fault.
func TestCmd_RefusesInconsistentFilters(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"until with follow", []string{"--follow", "--until", "1h"}, "--until cannot be combined with --follow"},
		{"unreadable since", []string{"--since", "yesterday"}, `--since: invalid time "yesterday"`},
		{"unreadable until", []string{"--until", "2026-13-01"}, `--until: invalid time "2026-13-01"`},
		{"empty window", []string{"--since", "2026-06-11T14:00:00Z", "--until", "2026-06-11T13:00:00Z"}, "--since (2026-06-11T14:00:00Z) is after --until"},
		{"empty grep term", []string{"--grep", "  "}, "--grep: an empty search term matches every line"},
		{"empty exclude term", []string{"--exclude", ""}, "--exclude: an empty search term matches every line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := Cmd()
			cmd.PreRunE = nil
			cmd.SetArgs(append([]string{"68b0c1d2e3f4a5b6c7d8e9f0"}, tc.args...))

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// The flags become the filter the client applies: every term of a repeated
// --grep and --exclude, the ids, and the window read relative to now.
func TestFilterFlags_Build(t *testing.T) {
	now := time.Date(2026, 6, 11, 14, 0, 0, 0, time.UTC)

	f := filterFlags{
		grep:    []string{"refused", "upstream"},
		exclude: []string{"healthz"},
		traceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		spanID:  "00f067aa0ba902b7",
		since:   "2h",
		until:   "2026-06-12",
	}

	filter, err := f.build("error", false, now)
	require.NoError(t, err)

	assert.Equal(t, "error", filter.Level)
	assert.Equal(t, []string{"refused", "upstream"}, filter.Grep)
	assert.Equal(t, []string{"healthz"}, filter.Exclude)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", filter.TraceID)
	assert.Equal(t, "00f067aa0ba902b7", filter.SpanID)
	assert.Equal(t, now.Add(-2*time.Hour), filter.Since)
	// A date as the closing bound is the whole of that day.
	assert.Equal(t, time.Date(2026, 6, 12, 23, 59, 59, 999_999_999, time.UTC), filter.Until)

	// So the same date on both sides is that day, not an empty window.
	f.since, f.until = "2026-06-12", "2026-06-12"

	filter, err = f.build("", false, now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), filter.Since)
	assert.True(t, filter.Until.After(filter.Since))
}

// An empty result under a filter is reported as such, not as a workload
// with no logs, and nothing reaches stderr under JSON. The unfiltered
// wording and the [] JSON prints are the renderer's, written straight to the
// process streams, and output_test.go holds them.
func TestCmd_EmptyResultNamesTheFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data": [], "count": 0, "next": null, "previous": null}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	run := func(args ...string) string {
		cmd := Cmd()
		cmd.PreRunE = nil
		cmd.SetArgs(append([]string{"68b0c1d2e3f4a5b6c7d8e9f0"}, args...))

		var stderr bytes.Buffer

		cmd.SetErr(&stderr)
		require.NoError(t, cmd.Execute())

		return stderr.String()
	}

	assert.Equal(t, "No logs matched the filters.\n", run("--grep", "refused"))
	assert.Equal(t, "No logs matched the filters.\n", run("--since", "1h"))
	assert.Empty(t, run("--grep", "refused", "--output-format", "json"), "JSON keeps stderr clean")
}
