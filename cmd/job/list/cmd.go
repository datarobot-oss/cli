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

package list

import (
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
		name   string
		limit  int
		offset int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs.",
		Long: `List the jobs you can see, newest first.

--name is an exact, case-sensitive match. Job names are not unique, so it can
return more than one job; a CI/CD pipeline that looks its job up by name should
expect exactly one.

Example:
  dr job list
  dr job list --name train
  dr job list --name train --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			jobs, err := job.ListJobs(name, limit, offset)
			if err != nil {
				return err
			}

			return job.RenderJobs(outputFormat, jobs)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&name, "name", "", "Only jobs with exactly this name")
	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of jobs to return")
	cmd.Flags().Var(countflags.NonNegativeInt(&offset, 0), "offset", "Number of jobs to skip before returning results")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputFormat),
			"name_provided": name != "",
			"limit":         limit,
			"offset":        offset,
		}
	})

	return cmd
}
