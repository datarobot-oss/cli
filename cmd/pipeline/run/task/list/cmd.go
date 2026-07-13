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
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		runID        string
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List task executions for a run",
		Long: `List per-@task execution records for a pipeline run.

Returns an empty list when the run has not yet received any task callbacks
(dispatch still in PENDING or PREPARING state).

Example:
  dr pipeline run task list --pipeline <id> --run <run-id>
  dr pipeline run task list --pipeline <id> --run <run-id> --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			tasks, err := pipeline.ListTaskExecutions(pipelineID, runID)
			if err != nil {
				return err
			}

			return renderTaskList(outputFormat, tasks)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	_ = cmd.MarkFlagRequired("pipeline")
	cmd.Flags().StringVar(&runID, "run", "", "Run (dispatch) ID")
	_ = cmd.MarkFlagRequired("run")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"run_id":        runID,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func renderTaskList(format outputformat.OutputFormat, tasks []pipeline.TaskExecution) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(tasks, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	if len(tasks) == 0 {
		fmt.Println(tui.DimStyle.Render("No task executions recorded yet"))

		return nil
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"TASK ID", "NAME", "STATUS", "STARTED", "COMPLETED"}

	completedCol := slices.Index(headers, "COMPLETED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == completedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, task := range tasks {
		taskIDStr := "-"
		if task.TaskID != nil {
			taskIDStr = strconv.Itoa(*task.TaskID)
		}

		startedStr := "-"
		if task.StartedAt != nil {
			startedStr = task.StartedAt.UTC().Format("2006-01-02 15:04:05")
		}

		completedStr := "-"
		if task.CompletedAt != nil {
			completedStr = task.CompletedAt.UTC().Format("2006-01-02 15:04:05")
		}

		t.Row(taskIDStr, task.Name, task.Status, startedStr, completedStr)
	}

	fmt.Fprintln(os.Stdout, t.Render())

	return nil
}
