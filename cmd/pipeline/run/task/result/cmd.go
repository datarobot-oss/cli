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

package result

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

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
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "result <task-id>",
		Short: "Get the presigned URL for a completed task's result",
		Long: `Return the presigned S3 URL for a completed @task's result blob,
plus a preview of the returned value.

The result is a cloudpickle payload. Download the URL directly from S3
and decode with cloudpickle.loads(). pipelines-api never proxies the bytes.

A text preview of the value is also shown: for JSON-serializable results
the structured value is displayed; for others (e.g. a DataFrame or numpy
array) a str() text preview is shown so you can eyeball the data without
downloading and unpickling the full object.

Returns 409 when the task has not yet reached COMPLETED state.

Example:
  dr pipeline run task result --pipeline <id> --run <run-id> 1
  dr pipeline run task result --pipeline <id> --run <run-id> 1 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			taskID, err := strconv.Atoi(args[0])
			if err != nil || taskID <= 0 {
				return fmt.Errorf("invalid task-id %q: must be a positive integer", args[0])
			}

			res, err := pipeline.GetTaskResult(pipelineID, runID, taskID)
			if err != nil {
				return err
			}

			return renderResult(cmd.OutOrStdout(), outputFormat, res)
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

func renderResult(w io.Writer, format outputformat.OutputFormat, res *pipeline.TaskExecutionResult) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}

		fmt.Fprintln(w, string(data))

		return nil
	}

	printResultHuman(w, res)

	return nil
}

func printResultHuman(out io.Writer, res *pipeline.TaskExecutionResult) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "URL:\t%s\n", res.URL)
	fmt.Fprintf(w, "Expires In:\t%ds\n", res.ExpiresIn)
	fmt.Fprintf(w, "Content Type:\t%s\n", res.ContentType)

	if res.ValueAvailable {
		valueStr := fmt.Sprintf("%v", res.Value)
		fmt.Fprintf(w, "Value Preview:\t%s\n", valueStr)
	} else {
		reason := "unavailable"
		if res.ValueUnavailableReason != nil {
			reason = *res.ValueUnavailableReason
		}

		fmt.Fprintf(w, "Value Preview:\t(not available: %s)\n", reason)
	}

	w.Flush()

	// When the JSON value can't represent the result (e.g. a DataFrame or
	// numpy array), fall back to the str() text preview the task pod
	// recorded. Printed outside the tabwriter so multi-line reprs keep
	// their own layout.
	if !res.ValueAvailable && res.ValueText != "" {
		header := "Text Preview:"
		if res.ValueTextTruncated {
			header = "Text Preview (truncated):"
		}

		fmt.Fprintf(out, "\n%s\n%s\n", header, res.ValueText)
	}
}
