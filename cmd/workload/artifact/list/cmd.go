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

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat workload.OutputFormat
		status       workload.Status
		limit        int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workload artifacts.",
		Long: `List workload artifacts in your DataRobot deployment infrastructure.

This command fetches artifacts and shows:
  * Name and current status (draft or locked)
  * Code reference catalog ID and version ID
  * Last update timestamp

By default, output is a human-readable table. Use --output-format json for machine-parseable output.

Example:
  dr workload artifact list
  dr workload artifact list --limit 10
  dr workload artifact list --status draft
  dr workload artifact list --output-format json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, _ []string) error {
			if limit <= 0 {
				return fmt.Errorf("invalid --limit %d: must be positive", limit)
			}

			artifacts, err := workload.ListArtifacts(limit, status)
			if err != nil {
				return err
			}

			return workload.RenderArtifacts(outputFormat, artifacts)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	workload.AddStatusFlag(cmd, &status)
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of artifacts to return")

	return cmd
}
