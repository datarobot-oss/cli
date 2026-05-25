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
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		mode         string
		offset       int
		limit        int
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipeline.",
		Long: `List pipelines registered with the pipelines service.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr pipeline list
  dr pipeline list --mode draft
  dr pipeline list --offset 0 --limit 50 --output-format json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, _ []string) error {
			if mode != "" && mode != pipeline.ModeDraft && mode != pipeline.ModeLocked {
				return fmt.Errorf("invalid mode: %s (supported: draft, locked)", mode)
			}

			list, err := pipeline.ListPipelines(mode, offset, limit)
			if err != nil {
				return err
			}

			return pipeline.RenderPipelines(outputFormat, *list)
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "Filter by mode: draft or locked")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 50, "Pagination limit (1-200)")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"mode":          mode,
			"offset":        offset,
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
