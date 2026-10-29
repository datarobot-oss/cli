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
	"time"
)

// WaitForRun polls the run on interval until it ends, timeout passes or ctx is
// cancelled. onTick, when set, sees every read. A run that ended is returned
// with no error whatever its status; Run.Succeeded says how it went. Giving up
// returns the last read with the error: the run itself keeps going.
func WaitForRun(
	ctx context.Context,
	jobID, runID string,
	interval, timeout time.Duration,
	onTick func(*Run),
) (*Run, error) {
	deadline := time.Now().Add(timeout)

	var last *Run

	for {
		if err := ctx.Err(); err != nil {
			return last, fmt.Errorf("stopped waiting for run %s: %w", runID, err)
		}

		run, err := GetRun(jobID, runID) //nolint:contextcheck // drapi takes no context
		if err != nil {
			return last, fmt.Errorf("poll run %s: %w", runID, err)
		}

		last = run

		if onTick != nil {
			onTick(run)
		}

		if run.Ended() {
			return run, nil
		}

		if time.Now().After(deadline) {
			return run, fmt.Errorf("timeout waiting for run %s after %s; it is still %s", runID, timeout, run.Status)
		}

		timer := time.NewTimer(interval)

		select {
		case <-ctx.Done():
			timer.Stop()

			return run, fmt.Errorf("stopped waiting for run %s: %w", runID, ctx.Err())
		case <-timer.C:
		}
	}
}

// Outcome is the error a run that ended without succeeding stands for, so a
// script that waited on it fails; nil for a run that succeeded.
func Outcome(run Run) error {
	if run.Succeeded() {
		return nil
	}

	if run.ExitCode != nil {
		return fmt.Errorf("run %s %s with exit code %d", run.ID, run.Status, *run.ExitCode)
	}

	return fmt.Errorf("run %s %s", run.ID, run.Status)
}
