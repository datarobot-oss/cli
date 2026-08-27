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
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		status       workload.Status
		limit        int
		offset       int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts.",
		Long: `List artifacts in your DataRobot deployment infrastructure.

This command fetches artifacts and shows:
  * Name and current status (draft or locked)
  * Code reference catalog ID and version ID
  * Last update timestamp

By default, output is a human-readable table. Use --output-format json for machine-parseable output.

Example:
  dr artifact list
  dr artifact list --limit 10
  dr artifact list --offset 100
  dr artifact list --offset 100 --limit 50
  dr artifact list --status draft
  dr artifact list --output-format json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			artifacts, err := workload.ListArtifacts(limit, offset, status)
			if err != nil {
				return err
			}

			return workload.RenderArtifacts(outputFormat, artifacts)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	workload.AddStatusFlag(cmd, &status)
	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of artifacts to return")
	cmd.Flags().Var(countflags.NonNegativeInt(&offset, 0), "offset", "Number of artifacts to skip before returning results")

	telemetry.TrackWith(cmd, func(c *cobra.Command, _ []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")
		offset, _ := c.Flags().GetInt("offset")

		return map[string]any{
			"limit":         limit,
			"offset":        offset,
			"output_format": string(outputFormat),
			"status":        string(status),
		}
	})

	return cmd
}
