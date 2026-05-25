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

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		flags        scopeflag.Flags
		offset       int
		limit        int
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipeline input sets",
		Long: `List input payloads for a pipeline.

Scope is selected the same way as create:
  - no flags                  -> draft
  - --version=N               -> locked, version N (scope auto-set)
  - --scope=draft             -> draft
  - --scope=locked --version=N -> locked, version N

Example:
  dr pipeline input list --pipeline <id>
  dr pipeline input list --pipeline <id> --version=2
  dr pipeline input list --pipeline <id> --offset 50 --limit 10 --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flags.PipelineID == "" {
				return errors.New("--pipeline is required")
			}

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			items, err := pipeline.ListInputs(flags.PipelineID, scope, version, offset, limit)
			if err != nil {
				return err
			}

			return pipeline.RenderInputs(outputFormat, items)
		},
	}

	flags.Bind(cmd)
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of inputs to return")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"pipeline_id":   flags.PipelineID,
			"scope":         flags.Scope,
			"version":       flags.Version,
			"offset":        offset,
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
