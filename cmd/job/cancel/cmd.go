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

package cancel

import (
	"fmt"

	"github.com/datarobot/cli/cmd/job/internal/jobargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/job"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "cancel <job-id> <run-id>",
		Short: "Cancel a run.",
		Long: `Cancel a run that has not ended. The run moves to stopping; once its pod
is gone it ends as stopped, or failed if its code exited with an error while
being stopped. A run that already ended cannot be cancelled.

Asks before cancelling; --yes skips the question.

Example:
  dr job cancel 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1
  dr job cancel 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1 --yes`,
		Args:         cobra.ExactArgs(2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jobID, runID := args[0], args[1]

			confirmed, err := jobargs.Confirm(cmd, fmt.Sprintf("Cancel run %s of job %s? [y/N] ", runID, jobID))
			if err != nil || !confirmed {
				return err
			}

			run, err := job.CancelRun(jobID, runID)
			if err != nil {
				return err
			}

			return job.RenderRun(outputFormat, *run)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	jobargs.AddYesFlag(cmd, "Cancel without asking")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat)}
	})

	return cmd
}
