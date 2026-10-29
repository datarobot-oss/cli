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
	"fmt"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/job"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// Runs start in seconds and are often short, so --wait polls more often than a
// build wait does; long training runs set their own --poll-timeout.
const (
	defaultPollInterval = 5 * time.Second
	defaultPollTimeout  = 2 * time.Hour
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		poll         pollflags.Set
	)

	cmd := &cobra.Command{
		Use:   "run <job-id>",
		Short: "Start a run of a job.",
		Long: `Start a run of a job with the artifact and runtime the job stores.

Without --wait the command returns once the run is accepted. With --wait it
follows the run until it ends, printing status changes to stderr, then prints
the run with its exit code and reason. A --wait run that does not succeed
exits non-zero, so a CI/CD step can gate on it.

Giving up on a wait (--poll-timeout or Ctrl-C) leaves the run going; cancel it
with 'dr job cancel'.

Example:
  dr job run 68b0c1d2e3f4a5b6c7d8e9f0
  dr job run 68b0c1d2e3f4a5b6c7d8e9f0 --wait
  dr job run 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json | jq -r .id`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jobID := args[0]

			started, err := job.TriggerRun(jobID)
			if err != nil {
				return err
			}

			if !poll.Wait {
				return job.RenderRun(outputFormat, *started)
			}

			return wait(cmd, outputFormat, jobID, started.ID, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	pollflags.RegisterWithDefaults(cmd, &poll, defaultPollInterval, defaultPollTimeout,
		"Follow the run until it ends; exit non-zero unless it succeeds.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat), "wait": poll.Wait}
	})

	return cmd
}

// wait follows the run to its end. Progress goes to stderr so stdout stays
// the run document, whatever the output format.
func wait(cmd *cobra.Command, format outputformat.OutputFormat, jobID, runID string, poll pollflags.Set) error {
	progress := cmd.ErrOrStderr()

	fmt.Fprintf(progress, "Started run %s. Waiting for it to end...\n", runID)

	lastStatus := ""

	run, err := job.WaitForRun(cmd.Context(), jobID, runID, poll.Interval, poll.Timeout, func(r *job.Run) {
		if r.Status != lastStatus {
			lastStatus = r.Status

			fmt.Fprintf(progress, "  %s\n", r.Status)
		}
	})
	if run == nil {
		return err
	}

	if renderErr := job.RenderRun(format, *run); renderErr != nil {
		return renderErr
	}

	if err != nil {
		return err
	}

	return job.Outcome(*run)
}
