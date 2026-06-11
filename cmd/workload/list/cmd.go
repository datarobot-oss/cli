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
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	var (
		limit    int
		statuses []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workloads.",
		Long: `List workloads in your DataRobot deployment infrastructure.

For each workload the listing shows:
  • Name and current status
  • Artifact type and importance
  • Last update timestamp

By default, output is a human-readable table. Use --output-format json for machine-parseable output.

Example:
  dr workload list
  dr workload list --limit 10
  dr workload list --status running
  dr workload list --status errored --status interrupted
  dr workload list --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if limit <= 0 {
				return fmt.Errorf("invalid --limit %d: must be positive", limit)
			}

			parsedStatuses, err := workload.ParseWorkloadStatuses(statuses)
			if err != nil {
				return err
			}

			workloads, err := workload.ListWorkloads(limit, parsedStatuses)
			if err != nil {
				return err
			}

			return workload.RenderWorkloads(outputFormat, workloads)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of workloads to return")
	cmd.Flags().StringSliceVar(&statuses, "status", nil,
		"Filter by status (repeatable, also accepts comma-separated values; e.g. running, errored)")

	telemetry.TrackWith(cmd, func(c *cobra.Command, _ []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")

		return map[string]any{
			"limit":         limit,
			"output_format": string(outputFormat),
			"status":        strings.Join(statuses, ","),
		}
	})

	return cmd
}
