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

package run

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runDoc = `{"id": "run-1", "jobId": "job-1", "artifactId": "art-1", "status": "%s", %s
	"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`

// serveRun accepts the trigger and answers each poll with the next status.
func serveRun(t *testing.T, ended string, statuses ...string) {
	t.Helper()

	polls := 0

	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/jobs/job-1/runs/":
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, runDoc, "submitted", "")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/jobs/job-1/runs/run-1/":
			status := statuses[min(polls, len(statuses)-1)]
			polls++

			extra := ""
			if polls >= len(statuses) {
				extra = ended
			}

			fmt.Fprintf(w, runDoc, status, extra)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
}

func execute(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var out, errOut bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func TestRun_WithoutWaitReturnsTheAcceptedRun(t *testing.T) {
	serveRun(t, "", "running")

	_, _, err := execute(t, "job-1", "--output-format", "json")

	require.NoError(t, err)
}

func TestRun_WaitFollowsTheRunToSuccess(t *testing.T) {
	serveRun(t, `"exitCode": 0, "terminationReason": "Completed",`, "submitted", "running", "succeeded")

	_, stderr, err := execute(t, "job-1", "--wait", "--poll-interval", "1ms")

	require.NoError(t, err)
	assert.Contains(t, stderr, "Started run run-1")
	assert.Contains(t, stderr, "  running\n")
	assert.Equal(t, 1, bytes.Count([]byte(stderr), []byte("  submitted\n")), "a status is narrated once")
}

func TestRun_WaitFailsWhenTheRunFails(t *testing.T) {
	serveRun(t, `"exitCode": 3, "terminationReason": "Error",`, "running", "failed")

	_, _, err := execute(t, "job-1", "--wait", "--poll-interval", "1ms")

	require.EqualError(t, err, "run run-1 failed with exit code 3")
}

func TestRun_WaitGivesUpAtTheTimeout(t *testing.T) {
	serveRun(t, "", "running")

	_, _, err := execute(t, "job-1", "--wait", "--poll-interval", "1ms", "--poll-timeout", "1ns")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "it is still running")
}
