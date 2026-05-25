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

package update

import (
	"errors"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		version      int
		cron         string
		timezone     string
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "update <schedule-id>",
		Short: "Update a pipeline schedule",
		Long: `Update the cron expression and/or timezone of an existing schedule.

At least one of --cron or --timezone must be supplied; otherwise the
command sends an empty patch which the API treats as a no-op.

Example:
  dr pipeline schedule update --pipeline <id> --version=2 <schedule-id> --cron "*/15 * * * *"
  dr pipeline schedule update --pipeline <id> --version=2 <schedule-id> --timezone Europe/Berlin`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := buildUpdateBody(cmd, pipelineID, version, cron, timezone)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdateSchedule(pipelineID, version, args[0], body)
			if err != nil {
				return err
			}

			return pipeline.RenderSchedule(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version")
	cmd.Flags().StringVar(&cron, "cron", "", "New cron expression")
	cmd.Flags().StringVar(&timezone, "timezone", "", "New IANA timezone name")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}

// buildUpdateBody validates the flag set and assembles the PATCH body. It is
// extracted from RunE to keep the cobra command's cyclomatic complexity low.
func buildUpdateBody(cmd *cobra.Command, pipelineID string, version int, cron, timezone string) (pipeline.ScheduleUpdateRequest, error) {
	if pipelineID == "" {
		return pipeline.ScheduleUpdateRequest{}, errors.New("--pipeline is required")
	}

	if version <= 0 {
		return pipeline.ScheduleUpdateRequest{}, errors.New("--version is required and must be > 0")
	}

	cronChanged := cmd.Flags().Changed("cron")
	tzChanged := cmd.Flags().Changed("timezone")

	if !cronChanged && !tzChanged {
		return pipeline.ScheduleUpdateRequest{}, errors.New("at least one of --cron or --timezone must be specified")
	}

	body := pipeline.ScheduleUpdateRequest{}

	if cronChanged {
		v := cron
		body.CronExpression = &v
	}

	if tzChanged {
		v := timezone
		body.Timezone = &v
	}

	return body, nil
}
