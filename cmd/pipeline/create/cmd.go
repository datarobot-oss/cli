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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		description  string
		mode         string
		outputFormat string
		fromFile     string
	)

	cmd := &cobra.Command{
		Use:   "create [<file>]",
		Short: "Upload a Python file to create a pipeline.",
		Long: `Upload a Python file containing a DataRobot pipeline (one or more tasks) to register a new pipeline.

The pipeline name is extracted from the file and used as the pipeline's resource name.
By default, output is human-readable. Use --output json for machine-parseable output.

The path to the Python file can be supplied either as a positional argument
or via the --from-file=<path> flag. Exactly one of the two must be provided.

Example:
  dr pipelines create ./my_pipeline.py
  dr pipelines create --from-file=./my_pipeline.py
  dr pipelines create ./my_pipeline.py --description "First draft" --mode draft
  dr pipelines create --from-file=./my_pipeline.py --output json`,
		Args:    cobra.MaximumNArgs(1),
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if mode != "" && mode != pipeline.ModeDraft && mode != pipeline.ModeLocked {
				return fmt.Errorf("invalid mode: %s (supported: draft, locked)", mode)
			}

			filePath, err := resolveFilePath(args, fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.CreatePipeline(filePath, description, mode)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return printCreateJSON(*result)
			}

			printCreateHuman(*result)

			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Optional description for the pipeline")
	cmd.Flags().StringVar(&mode, "mode", "", "Pipeline mode: draft (default) or locked")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the Python file to upload, e.g. --from-file=./my_pipeline.py (alternative to the positional argument)")

	return cmd
}

// resolveFilePath returns the file path supplied either positionally or via
// --from-file. Exactly one of the two must be provided.
func resolveFilePath(args []string, fromFile string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = args[0]
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

func printCreateJSON(result pipeline.CreateResponse) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printCreateHuman(result pipeline.CreateResponse) {
	tasks := "\u2014"
	if len(result.TaskNames) > 0 {
		tasks = strings.Join(result.TaskNames, ", ")
	}

	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID:  " + result.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Name:         " + result.Name))
	fmt.Println(tui.BaseTextStyle.Render("Version:      " + strconv.Itoa(result.Version)))
	fmt.Println(tui.BaseTextStyle.Render("Status:       " + result.Status))
	fmt.Println(tui.BaseTextStyle.Render("Mode:         " + result.Mode))
	fmt.Println(tui.BaseTextStyle.Render("Tasks:        " + tasks))
	fmt.Println(tui.DimStyle.Render("Created:      " + result.CreatedAt.UTC().Format(time.RFC3339)))
}
