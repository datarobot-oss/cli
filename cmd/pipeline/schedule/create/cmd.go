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

package create

import (
	"errors"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		version      int
		cron         string
		inputID      string
		imageID      string
		imageVersion int
		timezone     string
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a recurring schedule for a locked pipeline version",
		Long: `Register a cron-style schedule that triggers a run on a fixed cadence.

The schedule snapshots the pipeline version, input, and image at creation time
so that every future run uses the same reproducibility tuple.

Example:
  dr pipeline schedule create --pipeline <id> --version=2 --cron "0 * * * *" --input <input-id> --image <image-id> --image-version 1
  dr pipeline schedule create --pipeline <id> --version=2 --cron "0 9 * * *" --input <input-id> --image <image-id> --image-version 1 --timezone America/Los_Angeles`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			if version <= 0 {
				return errors.New("--version is required and must be > 0")
			}

			if imageVersion <= 0 {
				return errors.New("--image-version is required and must be > 0")
			}

			body := pipeline.ScheduleCreateRequest{
				CronExpression:    cron,
				PipelineVersionID: version,
				PipelineInputID:   inputID,
				ImageID:           imageID,
				ImageVersion:      imageVersion,
				Timezone:          timezone,
			}

			result, err := pipeline.CreateSchedule(pipelineID, body)
			if err != nil {
				return err
			}

			return pipeline.RenderSchedule(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	_ = cmd.MarkFlagRequired("pipeline")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version to schedule")
	_ = cmd.MarkFlagRequired("version")
	cmd.Flags().StringVar(&cron, "cron", "", "Cron expression, e.g. \"0 * * * *\"")
	_ = cmd.MarkFlagRequired("cron")
	cmd.Flags().StringVar(&inputID, "input", "", "Input ID to run on each tick")
	_ = cmd.MarkFlagRequired("input")
	cmd.Flags().StringVar(&imageID, "image", "", "Image ID to run the schedule with")
	_ = cmd.MarkFlagRequired("image")
	cmd.Flags().IntVar(&imageVersion, "image-version", 0, "Image version to run the schedule with")
	_ = cmd.MarkFlagRequired("image-version")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone name (default UTC)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"version":       version,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
