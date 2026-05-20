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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat string
		fromFile     string
	)

	cmd := &cobra.Command{
		Use:   "update <pipeline-id> [<file>]",
		Short: "Re-upload a Python file to update a draft pipeline.",
		Long: `Update an existing draft pipeline by re-uploading a Python file.

A new version is appended to the pipeline. The pipeline name encoded in the
uploaded file must match the existing pipeline name. Locked pipelines cannot
be updated.

The path to the Python file can be supplied either as a positional argument
or via the --from-file=<path> flag. Exactly one of the two must be provided.

By default, output is human-readable. Use --output json for machine-parseable output.

Example:
  dr pipelines update 507f1f77bcf86cd799439011 ./my_pipeline.py
  dr pipelines update 507f1f77bcf86cd799439011 --from-file=./my_pipeline.py
  dr pipelines update 507f1f77bcf86cd799439011 --from-file=./my_pipeline.py --output json`,
		Args:    cobra.RangeArgs(1, 2),
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			pipelineID := args[0]

			filePath, err := resolveFilePath(args[1:], fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdatePipeline(pipelineID, filePath)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return printUpdateJSON(*result)
			}

			printUpdateHuman(*result)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the Python file to upload, e.g. --from-file=./my_pipeline.py (alternative to the positional argument)")

	return cmd
}

// resolveFilePath returns the file path supplied either positionally (in
// extraArgs) or via --from-file. Exactly one of the two must be provided.
func resolveFilePath(extraArgs []string, fromFile string) (string, error) {
	positional := ""
	if len(extraArgs) > 0 {
		positional = extraArgs[0]
	}

	switch {
	case positional != "" && fromFile != "":
		return "", errors.New("specify the file either as a positional argument or via --from-file, not both")
	case positional != "":
		return positional, nil
	case fromFile != "":
		return fromFile, nil
	default:
		return "", errors.New("a file path is required (positional argument or --from-file)")
	}
}

func printUpdateJSON(result pipeline.CreateResponse) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printUpdateHuman(result pipeline.CreateResponse) {
	tasks := "\u2014"
	if len(result.TaskNames) > 0 {
		tasks = strings.Join(result.TaskNames, ", ")
	}

	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID:  " + result.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Name:         " + result.Name))
	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Version:      %d", result.Version)))
	fmt.Println(tui.BaseTextStyle.Render("Status:       " + result.Status))
	fmt.Println(tui.BaseTextStyle.Render("Mode:         " + result.Mode))
	fmt.Println(tui.BaseTextStyle.Render("Tasks:        " + tasks))
	fmt.Println(tui.DimStyle.Render("Updated:      " + result.CreatedAt.UTC().Format(time.RFC3339)))
}
