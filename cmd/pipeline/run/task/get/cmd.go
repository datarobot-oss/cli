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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
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
		Use:   "get <task-id>",
		Short: "Get a single task execution record",
		Long: `Display the lifecycle status of a single @task electron within a run.

<task-id> is the sequential task number (1, 2, 3, …) as returned by
"dr pipeline run task list".

Example:
  dr pipeline run task get --pipeline <id> --run <run-id> 1
  dr pipeline run task get --pipeline <id> --run <run-id> 2 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			task, err := pipeline.GetTaskExecution(pipelineID, runID, taskID)
			if err != nil {
				return handleNotFound(err, args[0])
			}

			return renderTask(outputFormat, task)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	_ = cmd.MarkFlagRequired("pipeline")
	cmd.Flags().StringVar(&runID, "run", "", "Run (dispatch) ID")
	_ = cmd.MarkFlagRequired("run")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"run_id":        runID,
			"task_id":       telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func parseTaskID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid task-id %q: must be a positive integer", s)
	}

	return id, nil
}

func renderTask(format outputformat.OutputFormat, task *pipeline.TaskExecution) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	printTaskHuman(task)

	return nil
}

func printTaskHuman(task *pipeline.TaskExecution) {
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

	errorStr := "-"
	if task.ErrorDetail != nil {
		errorStr = *task.ErrorDetail
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Task ID:\t%s\n", taskIDStr)
	fmt.Fprintf(w, "Name:\t%s\n", task.Name)
	fmt.Fprintf(w, "Status:\t%s\n", task.Status)
	fmt.Fprintf(w, "Started:\t%s\n", startedStr)
	fmt.Fprintf(w, "Completed:\t%s\n", completedStr)
	fmt.Fprintf(w, "Error:\t%s\n", errorStr)

	w.Flush()
}

func handleNotFound(err error, taskID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No task execution found with id: " + taskID))

		return nil
	}

	return err
}
