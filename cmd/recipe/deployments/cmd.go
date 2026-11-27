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

package deployments

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		statuses     []string
		limit        int
	)

	cmd := &cobra.Command{
		Use:   "deployments <recipe-id>",
		Short: "List the workloads deployed from a recipe.",
		Long: `List the llm workloads that run a recipe, with the recipe version each runs:
the version it was scheduled with, else the one it pinned, else "latest".

The Workload API has no recipe filter yet, so every llm workload in the tenant
is read and matched here.

Example:
  dr recipe deployments 6a1f0c2e9b7d4e3f2a1b0c9d
  dr recipe deployments 6a1f0c2e9b7d4e3f2a1b0c9d --status running`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			deps, err := llm.RecipeDeployments(args[0], statuses, limit)
			if err != nil {
				return err
			}

			return llm.RenderDeployments(cmd.OutOrStdout(), outputFormat, deps)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().StringSliceVar(&statuses, "status", nil, "Only workloads in these statuses (repeatable)")
	cmd.Flags().Var(countflags.PositiveInt(&limit, 50), "limit", "Maximum number of workloads to return")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"status_filter": len(statuses) > 0,
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
