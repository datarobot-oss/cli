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

package get

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "get <image-id>",
		Short: "Fetch details of a pipeline execution image",
		Long: `Fetch the full detail of a pipeline execution image, including all versions
and their build status.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr pipeline image get img-507f1f77bcf86cd799439011
  dr pipeline image get img-507f1f77bcf86cd799439011 --output-format json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, args []string) error {
			imageID := args[0]

			img, err := pipeline.GetImage(imageID)
			if err != nil {
				var httpErr *drapi.HTTPError

				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					fmt.Println(tui.DimStyle.Render("No image found: " + imageID))

					return nil
				}

				return err
			}

			return pipeline.RenderImage(outputFormat, *img)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"image_id":      telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
