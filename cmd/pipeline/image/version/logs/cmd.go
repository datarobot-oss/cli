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

package logs

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var imageID string

	cmd := &cobra.Command{
		Use:   "logs <version>",
		Short: "Fetch build logs for a specific pipeline execution image version",
		Long: `Fetch the raw build log output for a specific pipeline execution image version.

Logs are available once the build has completed (status READY or ERROR).

Example:
  dr pipeline image version logs --image img-507f1f77bcf86cd799439011 1`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := strconv.Atoi(args[0])
			if err != nil || version <= 0 {
				return fmt.Errorf("invalid version: %q (expected a positive integer)", args[0])
			}

			resp, err := pipeline.GetImageBuildLogs(imageID, version)
			if err != nil {
				var httpErr *drapi.HTTPError

				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					fmt.Println(tui.DimStyle.Render(
						fmt.Sprintf("No image version found: %s v%d", imageID, version),
					))

					return nil
				}

				return err
			}

			fmt.Print(resp.Logs)

			return nil
		},
	}

	cmd.Flags().StringVar(&imageID, "image", "", "Image ID (required)")
	_ = cmd.MarkFlagRequired("image")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"image_id": imageID,
			"version":  telemetry.FirstArg(args),
		}
	})

	return cmd
}
