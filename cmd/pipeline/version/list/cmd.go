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

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		offset       int
		limit        int
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List versions of a pipeline",
		Long: `List versions of a pipeline (paginated).

Example:
  dr pipeline version list --pipeline <id>
  dr pipeline version list --pipeline <id> --offset 10 --limit 5 --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			items, err := pipeline.ListVersions(pipelineID, offset, limit)
			if err != nil {
				return err
			}

			return pipeline.RenderVersions(outputFormat, items)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of versions to return")
	pipelines.AddOutputFlag(cmd, &outputFormat)

	return cmd
}
