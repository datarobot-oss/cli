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
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat pipeline.OutputFormat

	cmd := &cobra.Command{
		Use:   "get <pipeline-id>",
		Short: "Display details of a pipeline.",
		Long: `Display full details of a pipeline including all versions.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr pipeline get 507f1f77bcf86cd799439011
  dr pipeline get 507f1f77bcf86cd799439011 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			result, err := pipeline.GetPipeline(args[0])
			if err != nil {
				return handleGetError(err, args[0])
			}

			return pipeline.RenderPipeline(outputFormat, *result)
		},
	}

	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

// handleGetError translates a GetPipeline error into a user-facing message.
// A 404 is rendered as a friendly "No pipeline found" line on stdout and
// suppressed (returns nil) so the user does not see an HTTP-style stack
// or the command's usage on what is really an informational outcome.
func handleGetError(err error, pipelineID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No pipeline found with id: " + pipelineID))

		return nil
	}

	return err
}
