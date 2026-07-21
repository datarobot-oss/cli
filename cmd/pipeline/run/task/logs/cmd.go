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

package logs

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		runID        string
		stream       string
		tailLines    int
		nodeID       int
		verbosity    string
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "logs <task-id>",
		Short: "Fetch logs for a task execution",
		Long: `Fetch logs for an individual @task electron in a run.

Without --stream: reads live K8s pod logs (stops working ~60s after the
job terminates when kubelet GC's the pod).

With --stream stdout or --stream stderr: reads the durable S3-uploaded
log captured by the electron runner at process exit. Use this for
post-mortem debugging after the pod has been GC'd.

When the same @task runs at multiple graph nodes (fan-out), all invocations
share one <task-id>. Pass --node-id (the NODE ID column from
"dr pipeline run task list") to read a specific invocation's logs; without it
an ambiguous task returns an error listing the candidate node ids.

Example:
  dr pipeline run task logs --pipeline <id> --run <run-id> 1
  dr pipeline run task logs --pipeline <id> --run <run-id> 3 --node-id 7 --stream stderr
  dr pipeline run task logs --pipeline <id> --run <run-id> 1 --tail 100 --verbosity all`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			taskID, err := strconv.Atoi(args[0])
			if err != nil || taskID <= 0 {
				return fmt.Errorf("invalid task-id %q: must be a positive integer", args[0])
			}

			if stream != "" && stream != "stdout" && stream != "stderr" {
				return fmt.Errorf("--stream must be stdout or stderr (got %q)", stream)
			}

			var node *int
			if cmd.Flags().Changed("node-id") {
				node = &nodeID
			}

			if stream != "" {
				return fetchDurableLog(pipelineID, runID, taskID, node, stream, verbosity, outputFormat)
			}

			var tail *int

			if cmd.Flags().Changed("tail") {
				tail = &tailLines
			}

			return fetchLiveLogs(pipelineID, runID, taskID, node, tail, verbosity, outputFormat)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	_ = cmd.MarkFlagRequired("pipeline")
	cmd.Flags().StringVar(&runID, "run", "", "Run (dispatch) ID")
	_ = cmd.MarkFlagRequired("run")
	cmd.Flags().StringVar(&stream, "stream", "", "Read durable S3 log: stdout or stderr")
	cmd.Flags().IntVar(&tailLines, "tail", 0, "Limit to last N lines (live logs only)")
	cmd.Flags().IntVar(&nodeID, "node-id", 0, "Select a specific fan-out invocation by its nodeId (from `task list`)")
	cmd.Flags().StringVar(&verbosity, "verbosity", "", "Log verbosity: user (default) or all")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"run_id":        runID,
			"task_id":       telemetry.FirstArg(args),
			"stream":        stream,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func fetchLiveLogs(pipelineID, runID string, taskID int, nodeID, tailLines *int, verbosity string, format outputformat.OutputFormat) error {
	logs, err := pipeline.GetTaskLogs(pipelineID, runID, taskID, nodeID, tailLines, verbosity)
	if err != nil {
		return err
	}

	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(logs, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	fmt.Print(logs.Logs)

	return nil
}

func fetchDurableLog(pipelineID, runID string, taskID int, nodeID *int, stream, verbosity string, format outputformat.OutputFormat) error {
	log, err := pipeline.GetTaskDurableLog(pipelineID, runID, taskID, nodeID, stream, verbosity)
	if err != nil {
		return err
	}

	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(log, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	fmt.Print(log.Content)

	return nil
}
