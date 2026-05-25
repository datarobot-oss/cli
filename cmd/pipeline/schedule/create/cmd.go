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
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		version      int
		cron         string
		inputID      string
		timezone     string
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a recurring schedule for a locked pipeline version",
		Long: `Register a cron-style schedule that triggers a run on a fixed cadence.

Example:
  dr pipeline schedule create --pipeline <id> --version=2 --cron "0 * * * *" --input <input-id>
  dr pipeline schedule create --pipeline <id> --version=2 --cron "0 9 * * *" --input <input-id> --timezone America/Los_Angeles`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			if version <= 0 {
				return errors.New("--version is required and must be > 0")
			}

			if cron == "" {
				return errors.New("--cron is required")
			}

			if inputID == "" {
				return errors.New("--input is required")
			}

			body := pipeline.ScheduleCreateRequest{
				CronExpression:  cron,
				PipelineInputID: inputID,
				Timezone:        timezone,
			}

			result, err := pipeline.CreateSchedule(pipelineID, version, body)
			if err != nil {
				return err
			}

			return pipeline.RenderSchedule(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version")
	cmd.Flags().StringVar(&cron, "cron", "", "Cron expression, e.g. \"0 * * * *\"")
	cmd.Flags().StringVar(&inputID, "input", "", "Input ID to run on each tick")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone name (default UTC)")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}
