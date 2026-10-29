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

package del

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
		Use:     "delete <job-id>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a job.",
		Long: `Delete a job. Its active runs are stopped first.

The job's artifact is not deleted; remove it with 'dr artifact delete'.

Asks before deleting; --yes skips the question.

Example:
  dr job delete 68b0c1d2e3f4a5b6c7d8e9f0
  dr job delete 68b0c1d2e3f4a5b6c7d8e9f0 --yes`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jobID := args[0]

			confirmed, err := jobargs.Confirm(cmd, fmt.Sprintf(
				"Delete job %s? Its active runs are stopped. This cannot be undone. [y/N] ", jobID))
			if err != nil || !confirmed {
				return err
			}

			if err := job.DeleteJob(jobID); err != nil {
				return err
			}

			if outputFormat == outputformat.OutputFormatJSON {
				return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "deleted", map[string]string{"id": jobID})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted job %s.\n", jobID)

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	jobargs.AddYesFlag(cmd, "Delete without asking")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat)}
	})

	return cmd
}
