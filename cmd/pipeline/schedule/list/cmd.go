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
	"errors"
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/schedule/scheduleutil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		version      int
		offset       int
		limit        int
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List schedules for a locked pipeline version",
		Long: `List recurring schedules attached to a locked pipeline version.

Example:
  dr pipelines schedule list --pipeline <id> --version=2
  dr pipelines schedule list --pipeline <id> --version=2 --output json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			if version <= 0 {
				return errors.New("--version is required and must be > 0")
			}

			items, err := pipeline.ListSchedules(pipelineID, version, offset, limit)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return scheduleutil.PrintScheduleListJSON(items)
			}

			scheduleutil.PrintScheduleListHuman(items)

			return nil
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().IntVar(&version, "version", 0, "Locked pipeline version")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of schedules to return")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}
