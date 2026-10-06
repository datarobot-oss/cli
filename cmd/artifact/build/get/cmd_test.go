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

package get

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveFailedBuild answers the build's log stream with logs and the build
// itself as FAILED, after runningFirst reads of it as RUNNING, so the test
// controls the only thing the failure message is allowed to claim.
func serveFailedBuild(t *testing.T, runningFirst int, logs func(w http.ResponseWriter)) {
	t.Helper()

	var reads atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v2/otel/artifact/art-1/logs/"):
			logs(w)
		case strings.TrimSuffix(r.URL.Path, "/") == "/api/v2/artifacts/art-1/builds/b-1":
			status := "FAILED"
			if int(reads.Add(1)) <= runningFirst {
				status = "RUNNING"
			}

			fmt.Fprintf(w, `{"id":"b-1","artifactId":"art-1","status":%q,"failureReason":"step 3 exited 1"}`, status)
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(srv.Close)
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)
	t.Cleanup(viperx.Reset)
}

// A build that failed before --wait had anything to wait for gets the same
// error as one that fails during the wait, and that error names the logs
// only when there are some.
func TestCmd_WaitOnAFailedBuildNamesTheLogsOnlyWhenThereAreSome(t *testing.T) {
	lines := func(w http.ResponseWriter) {
		fmt.Fprint(w, `{"data":[{"timestamp":"2026-10-02T10:00:00Z","level":"error","message":"step 3 exited 1"}],"count":1,"next":""}`)
	}

	for _, c := range []struct {
		name         string
		runningFirst int
		logs         func(w http.ResponseWriter)
		want         string
	}{
		{name: "already failed, the stream has lines", logs: lines, want: "see 'dr artifact build logs art-1 b-1'"},
		{
			name: "already failed, the stream is empty",
			logs: func(w http.ResponseWriter) { fmt.Fprint(w, `{"data":[],"count":0,"next":""}`) },
			want: "no log lines have been captured",
		},
		{
			name: "already failed, the stream cannot be read",
			logs: func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) },
			want: "could not be read just now",
		},
		{name: "fails during the wait", runningFirst: 1, logs: lines, want: "see 'dr artifact build logs art-1 b-1'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			serveFailedBuild(t, c.runningFirst, c.logs)

			var stderr bytes.Buffer

			cmd := Cmd()
			cmd.PreRunE = nil
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"art-1", "b-1", "--wait", "--poll-interval", "5ms"})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ended with status FAILED")
			assert.Contains(t, err.Error(), c.want)
			assert.Equal(t, c.runningFirst > 0, strings.Contains(stderr.String(), "Waiting for build"),
				"only a build still running is waited for")
		})
	}
}

func TestCmd_RequiresAtLeastOneArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_RejectsThirdArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "extra"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "b-1", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_ListsPollFlags(t *testing.T) {
	cmd := Cmd()

	pollIntervalFlag := cmd.Flag("poll-interval")
	pollTimeoutFlag := cmd.Flag("poll-timeout")

	require.NotNil(t, pollIntervalFlag)
	require.NotNil(t, pollTimeoutFlag)
	assert.False(t, pollIntervalFlag.Hidden)
	assert.False(t, pollTimeoutFlag.Hidden)
}
