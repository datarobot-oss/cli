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
	var (
		pipelineID   string
		version      int
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "get <schedule-id>",
		Short: "Display details of a pipeline schedule",
		Long: `Display the cron expression, timezone, and lifecycle status of a schedule.

Example:
  dr pipeline schedule get --pipeline <id> --version=2 <schedule-id>
  dr pipeline schedule get --pipeline <id> --version=2 <schedule-id> --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if version <= 0 {
				return errors.New("--version is required and must be > 0")
			}

			result, err := pipeline.GetSchedule(pipelineID, version, args[0])
			if err != nil {
				return handleGetError(err, args[0])
			}

			return pipeline.RenderSchedule(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	_ = cmd.MarkFlagRequired("pipeline")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"schedule_id":   telemetry.FirstArg(args),
			"version":       version,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func handleGetError(err error, scheduleID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No schedule found with id: " + scheduleID))

		return nil
	}

	return err
}
