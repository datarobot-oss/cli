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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
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

	settleWithin(t, 50*time.Millisecond)
}

// settleWithin bounds how long a failed build's logs are waited for.
func settleWithin(t *testing.T, budget time.Duration) {
	t.Helper()

	prev := logSettleBudget
	logSettleBudget = budget

	t.Cleanup(func() { logSettleBudget = prev })
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
			assert.Equal(t, c.runningFirst > 0, strings.Contains(stderr.String(), "waiting up to a minute"),
				"only a failure the wait saw happen has lines still on their way")
		})
	}
}

// A failure the wait saw happen is read until the builder's closing line has
// arrived, so the summary's logTail carries the cause rather than nothing.
func TestCmd_WaitThatSeesTheFailureReadsUntilItsLastLines(t *testing.T) {
	var reads atomic.Int32

	serveFailedBuild(t, 1, func(w http.ResponseWriter) {
		if reads.Add(1) <= 2 {
			fmt.Fprint(w, `{"data":[],"count":0,"next":""}`)

			return
		}

		fmt.Fprint(w, `{"data":[
			{"timestamp":"2026-10-02T10:00:03Z","level":"info","message":"Image build FAILED in 3.069211 seconds."},
			{"timestamp":"2026-10-02T10:00:02Z","level":"error","message":"failed to solve: exit code: 7"}
		],"count":2,"next":""}`)
	})
	settleWithin(t, time.Minute)

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"art-1", "b-1", "--wait", "--poll-interval", "1ms", "--output-format", "json"})

	var err error

	stdout := testutil.CaptureStdout(t, func() { err = cmd.Execute() })

	require.Error(t, err)
	assert.Contains(t, err.Error(), "see 'dr artifact build logs art-1 b-1'")

	var summary struct {
		LogTail []struct {
			Message string `json:"message"`
		} `json:"logTail"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &summary))
	require.Len(t, summary.LogTail, 2)
	assert.Equal(t, "failed to solve: exit code: 7", summary.LogTail[0].Message)
	assert.Equal(t, "Image build FAILED in 3.069211 seconds.", summary.LogTail[1].Message)
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
