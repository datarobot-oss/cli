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

package create

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

// serveFailingBuild accepts the trigger, reports the build FAILED on its
// first poll and answers the log stream with logs, so the test controls the
// only thing the failure message is allowed to claim.
func serveFailingBuild(t *testing.T, logs func(w http.ResponseWriter)) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v2/otel/artifact/art-1/logs/"):
			logs(w)
		case r.Method == http.MethodPost && strings.TrimSuffix(r.URL.Path, "/") == "/api/v2/artifacts/art-1/builds":
			fmt.Fprint(w, `{"buildIds":["b-1"]}`)
		case strings.TrimSuffix(r.URL.Path, "/") == "/api/v2/artifacts/art-1/builds/b-1":
			fmt.Fprint(w, `{"id":"b-1","artifactId":"art-1","status":"FAILED","failureReason":"step 3 exited 1"}`)
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

// A --wait that ends on a failed build names the logs only when there are
// some: lines the stream printed count, an empty stream says so, and a
// stream that cannot be read is not called empty.
func TestCmd_WaitOnAFailedBuildNamesTheLogsOnlyWhenThereAreSome(t *testing.T) {
	for _, c := range []struct {
		name string
		logs func(w http.ResponseWriter)
		want string
	}{
		{
			// The stream answers the tail's reads and nothing after, so the
			// lines the tail printed are the only evidence there is.
			name: "the stream has lines",
			logs: func() func(w http.ResponseWriter) {
				var reads atomic.Int32

				return func(w http.ResponseWriter) {
					if reads.Add(1) > 2 {
						w.WriteHeader(http.StatusBadGateway)

						return
					}

					fmt.Fprint(w, `{"data":[{"timestamp":"2026-10-02T10:00:00Z","level":"error","message":"step 3 exited 1"}],"count":1,"next":""}`)
				}
			}(),
			want: "see 'dr artifact build logs art-1 b-1'",
		},
		{
			name: "the stream is empty",
			logs: func(w http.ResponseWriter) { fmt.Fprint(w, `{"data":[],"count":0,"next":""}`) },
			want: "no log lines have been captured",
		},
		{
			name: "the stream cannot be read",
			logs: func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadGateway) },
			want: "could not be read just now",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			serveFailingBuild(t, c.logs)

			var stderr bytes.Buffer

			cmd := Cmd()
			cmd.PreRunE = nil
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"art-1", "--wait"})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "build b-1 ended with status FAILED")
			assert.Contains(t, err.Error(), c.want)
			assert.Contains(t, stderr.String(), "Waiting for build b-1")
		})
	}
}

func TestCmd_RejectsExtraArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "extra"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"art-1", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output format")
}

func TestCmd_ParsesPollFlags(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil

	require.NoError(t, cmd.ParseFlags([]string{"--wait", "--poll-interval", "100ms", "--poll-timeout", "30s"}))

	wait, _ := cmd.Flags().GetBool("wait")
	interval, _ := cmd.Flags().GetDuration("poll-interval")
	timeout, _ := cmd.Flags().GetDuration("poll-timeout")

	assert.True(t, wait)
	assert.Equal(t, "100ms", interval.String())
	assert.Equal(t, "30s", timeout.String())
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
