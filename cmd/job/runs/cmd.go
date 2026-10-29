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

package runs

import (
	"errors"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/job"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		statuses []string
		limit    int
		offset   int
	)

	cmd := &cobra.Command{
		Use:   "runs <job-id> [<run-id>]",
		Short: "List a job's runs, or display one.",
		Long: `List a job's runs, newest first, with how each one ended. Given a run ID,
display that run instead.

--status keeps only runs in the given statuses; repeat it or separate values
with commas. Statuses: submitted, provisioning, launching, running, stopping,
succeeded, failed, errored, stopped.

Example:
  dr job runs 68b0c1d2e3f4a5b6c7d8e9f0
  dr job runs 68b0c1d2e3f4a5b6c7d8e9f0 --status failed,errored
  dr job runs 68b0c1d2e3f4a5b6c7d8e9f0 68b0c1d2e3f4a5b6c7d8e9f1`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			jobID := args[0]

			if len(args) == 2 {
				if cmd.Flags().Changed("status") || cmd.Flags().Changed("limit") || cmd.Flags().Changed("offset") {
					return errors.New("--status, --limit and --offset list runs; drop them to display one run")
				}

				run, err := job.GetRun(jobID, args[1])
				if err != nil {
					return err
				}

				return job.RenderRun(outputFormat, *run)
			}

			parsed, err := job.ParseRunStatuses(statuses)
			if err != nil {
				return err
			}

			runs, err := job.ListRuns(jobID, parsed, limit, offset)
			if err != nil {
				return err
			}

			return job.RenderRuns(outputFormat, runs)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringSliceVar(&statuses, "status", nil, "Only runs in these statuses")
	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of runs to return")
	cmd.Flags().Var(countflags.NonNegativeInt(&offset, 0), "offset", "Number of runs to skip before returning results")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"output_format":   string(outputFormat),
			"run_id_provided": len(args) == 2,
			"status_count":    len(statuses),
			"limit":           limit,
			"offset":          offset,
		}
	})

	return cmd
}
