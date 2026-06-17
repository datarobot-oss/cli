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
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		offset       int
		limit        int
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipeline execution images",
		Long: `List pipeline execution images.

Returns a tabular view of registered images, newest first. Each row
reflects the latest version's status only; per-version details are
returned by ` + "`image create`" + ` and ` + "`image update`" + `.

Example:
  dr pipeline image list
  dr pipeline image list --offset 50 --limit 10 --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			items, err := pipeline.ListImages(offset, limit)
			if err != nil {
				return err
			}

			return pipeline.RenderImages(outputFormat, items)
		},
	}

	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of images to return")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"offset":        offset,
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
