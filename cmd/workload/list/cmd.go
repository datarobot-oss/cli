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
	"errors"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		limit    int
		offset   int
		statuses []string
		enclave  string
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

Use --enclave to only list workloads running on the named Enclave. The
filter matches where each workload actually runs (its resolved placement),
not the pin requested at create time; an unknown name simply matches
nothing.

Example:
  dr workload list
  dr workload list --limit 10
  dr workload list --offset 100
  dr workload list --offset 100 --limit 50
  dr workload list --status running
  dr workload list --status errored --status interrupted
  dr workload list --enclave prod-east
  dr workload list --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			// A blank --enclave would silently drop the filter and list
			// everything, the opposite of what the flag asked for.
			if cmd.Flags().Changed("enclave") && strings.TrimSpace(enclave) == "" {
				return errors.New("invalid --enclave: the Enclave name must be non-blank")
			}

			parsedStatuses, err := workload.ParseWorkloadStatuses(statuses)
			if err != nil {
				return err
			}

			workloads, err := workload.ListWorkloads(limit, offset, parsedStatuses, enclave)
			if err != nil {
				return err
			}

			return workload.RenderWorkloads(outputFormat, workloads)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().Var(countflags.PositiveInt(&limit, 100), "limit", "Maximum number of workloads to return")
	cmd.Flags().Var(countflags.NonNegativeInt(&offset, 0), "offset", "Number of workloads to skip before returning results")
	cmd.Flags().StringSliceVar(&statuses, "status", nil,
		"Filter by status (repeatable, also accepts comma-separated values; e.g. running, errored)")
	cmd.Flags().StringVar(&enclave, "enclave", "",
		"Only list workloads running on the named Enclave")

	telemetry.TrackWith(cmd, func(c *cobra.Command, _ []string) map[string]any {
		limit, _ := c.Flags().GetInt("limit")
		offset, _ := c.Flags().GetInt("offset")

		return map[string]any{
			"limit":         limit,
			"offset":        offset,
			"output_format": string(outputFormat),
			"status":        strings.Join(statuses, ","),
			"enclave":       enclave,
		}
	})

	return cmd
}
