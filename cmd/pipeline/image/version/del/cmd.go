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

// Package del implements `dr pipeline image version delete`.
// Directory is named `del` to avoid shadowing Go's built-in `delete()`.

package del

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
		Use:   "delete <version>",
		Short: "Delete a specific version of a pipeline execution image",
		Long: `Soft-delete a specific version of a pipeline execution image
without touching the parent image.

Example:
  dr pipeline image version delete --image img-123 2`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			version, err := strconv.Atoi(args[0])
			if err != nil || version <= 0 {
				return fmt.Errorf("invalid version: %q (expected a positive integer)", args[0])
			}

			err = pipeline.DeleteImageVersion(imageID, version)
			if err != nil {
				return handleDeleteError(err, imageID, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render(
				fmt.Sprintf("Deleted image version: %s v%d", imageID, version),
			))

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

// handleDeleteError converts a 404 into a friendly informational message
// (returns nil) so the user does not see a stack-trace-style HTTP error
// for what is effectively a no-op.
func handleDeleteError(err error, imageID, version string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render(
			fmt.Sprintf("No image version found: %s v%s", imageID, version),
		))

		return nil
	}

	return err
}
