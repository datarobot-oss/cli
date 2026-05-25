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

// Package del implements the `dr pipeline schedule delete` verb. The
// directory is named `del` rather than `delete` because the latter
// shadows Go's built-in delete() function in importing files.

package del

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID string
		version    int
	)

	cmd := &cobra.Command{
		Use:   "delete <schedule-id>",
		Short: "Delete a pipeline schedule",
		Long: `Delete a recurring schedule from a locked pipeline version.

Example:
  dr pipeline schedule delete --pipeline <id> --version=2 <schedule-id>`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			if version <= 0 {
				return errors.New("--version is required and must be > 0")
			}

			err := pipeline.DeleteSchedule(pipelineID, version, args[0])
			if err != nil {
				return err
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted schedule: " + args[0]))

			return nil
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id": pipelineID,
			"schedule_id": telemetry.FirstArg(args),
			"version":     version,
		}
	})

	return cmd
}
