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
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/version/versionutil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		offset       int
		limit        int
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List versions of a pipeline",
		Long: `List versions of a pipeline (paginated).

Example:
  dr pipelines version list --pipeline <id>
  dr pipelines version list --pipeline <id> --offset 10 --limit 5 --output json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			items, err := pipeline.ListVersions(pipelineID, offset, limit)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return versionutil.PrintVersionListJSON(items)
			}

			versionutil.PrintVersionListHuman(items)

			return nil
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of versions to return")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}
