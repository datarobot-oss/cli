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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "get <pipeline-id>",
		Short: "Display details of a pipeline.",
		Long: `Display full details of a pipeline including all versions.

By default, output is human-readable. Use --output json for machine-parseable output.

Example:
  dr pipelines get 507f1f77bcf86cd799439011
  dr pipelines get 507f1f77bcf86cd799439011 --output json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			pipeline, err := pipeline.GetPipeline(args[0])
			if err != nil {
				return handleGetError(err, args[0])
			}

			if outputFormat == "json" {
				return printGetJSON(*pipeline)
			}

			printGetHuman(*pipeline)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}

// handleGetError translates a GetPipeline error into a user-facing message.
// A 404 is rendered as a friendly "No pipeline found" line on stdout and
// suppressed (returns nil) so the user does not see an HTTP-style stack
// or the command's usage on what is really an informational outcome.
func handleGetError(err error, pipelineID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No pipeline found with id: " + pipelineID))

		return nil
	}

	return err
}

func printGetJSON(pipeline pipeline.Pipeline) error {
	data, err := json.MarshalIndent(pipeline, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printGetHuman(pipeline pipeline.Pipeline) {
	description := "\u2014"
	if pipeline.Description != "" {
		description = pipeline.Description
	}

	fmt.Println(tui.BaseTextStyle.Render("ID:          " + pipeline.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Name:        " + pipeline.Name))
	fmt.Println(tui.BaseTextStyle.Render("Description: " + description))
	fmt.Println(tui.BaseTextStyle.Render("Mode:        " + pipeline.Mode))
	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Active:      %t", pipeline.IsActive)))
	fmt.Println(tui.DimStyle.Render("Created:     " + pipeline.CreatedAt.UTC().Format(time.RFC3339)))
	fmt.Println(tui.DimStyle.Render("Updated:     " + pipeline.UpdatedAt.UTC().Format(time.RFC3339)))

	if len(pipeline.Versions) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Versions (%d):", len(pipeline.Versions))))

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "  VERSION\tSTATUS\tPYTHON\tCREATED\tTASKS")

	for _, version := range pipeline.Versions {
		tasks := "\u2014"
		if len(version.TaskNames) > 0 {
			tasks = strings.Join(version.TaskNames, ", ")
		}

		python := version.PythonVersion
		if python == "" {
			python = "\u2014"
		}

		fmt.Fprintf(writer, "  v%s\t%s\t%s\t%s\t%s\n",
			strconv.Itoa(version.Version),
			version.Status,
			python,
			version.CreatedAt.UTC().Format(time.RFC3339),
			tasks,
		)
	}

	_ = writer.Flush()

	for _, version := range pipeline.Versions {
		if version.ErrorDetail == "" {
			continue
		}

		fmt.Println(tui.DimStyle.Render(fmt.Sprintf("  v%d error: %s", version.Version, version.ErrorDetail)))
	}
}
