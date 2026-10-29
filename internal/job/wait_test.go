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

package job

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForRun_PollsUntilTheRunEnds(t *testing.T) {
	statuses := []string{StatusSubmitted, StatusRunning, StatusFailed}
	polls := 0

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job-1/runs/run-1/", r.URL.Path)

		fmt.Fprintf(w, runDoc, statuses[min(polls, len(statuses)-1)])

		polls++
	})

	var seen []string

	run, err := WaitForRun(context.Background(), "job-1", "run-1", time.Millisecond, time.Minute,
		func(r *Run) { seen = append(seen, r.Status) })

	require.NoError(t, err, "a run that ended is not an error to wait on")
	assert.Equal(t, StatusFailed, run.Status)
	assert.Equal(t, statuses, seen)
}

func TestWaitForRun_GivesUpAtTheTimeout(t *testing.T) {
	serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, runDoc, StatusRunning)
	})

	run, err := WaitForRun(context.Background(), "job-1", "run-1", time.Millisecond, time.Nanosecond, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "it is still running")
	assert.Equal(t, StatusRunning, run.Status, "the last read comes back with the error")
}

func TestWaitForRun_StopsWhenCancelled(t *testing.T) {
	serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, runDoc, StatusRunning)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WaitForRun(ctx, "job-1", "run-1", time.Millisecond, time.Minute, nil)

	require.ErrorIs(t, err, context.Canceled)
}

func TestOutcome(t *testing.T) {
	three := 3

	require.NoError(t, Outcome(Run{ID: "run-1", Status: StatusSucceeded}))
	require.EqualError(t, Outcome(Run{ID: "run-1", Status: StatusFailed, ExitCode: &three}),
		"run run-1 failed with exit code 3")
	require.EqualError(t, Outcome(Run{ID: "run-1", Status: StatusStopped}), "run run-1 stopped")
}
