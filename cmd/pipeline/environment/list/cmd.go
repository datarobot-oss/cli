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
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		offset       int
		limit        int
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipeline execution environments",
		Long: `List pipeline execution environments.

Returns a tabular view of registered environments, newest first. Each
row reflects the latest version's status only; per-version details are
returned by ` + "`environment create`" + ` and ` + "`environment update`" + `.

Example:
  dr pipeline environment list
  dr pipeline environment list --offset 50 --limit 10 --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			items, err := pipeline.ListEnvironments(offset, limit)
			if err != nil {
				return err
			}

			return pipeline.RenderEnvironments(outputFormat, items)
		},
	}

	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of environments to return")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}
