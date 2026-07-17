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

// task_output.go holds the rendering helpers for dr pipeline task verbs.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

// taskParamJSON is the CLI-facing shape for a single task parameter.
type taskParamJSON struct {
	Name       string  `json:"name"`
	Annotation *string `json:"annotation,omitempty"`
}

// taskJSON is the CLI-facing DTO used for `--output-format json`. It remaps
// camelCase wire keys to snake_case to match the other pipeline output DTOs.
type taskJSON struct {
	TaskID         int             `json:"task_id"`
	PipelineID     string          `json:"pipeline_id"`
	VersionID      *int            `json:"version_id,omitempty"`
	Name           string          `json:"name"`
	Parameters     []taskParamJSON `json:"parameters"`
	Inputs         map[string]any  `json:"inputs,omitempty"`
	Source         string          `json:"source"`
	ResourceBundle map[string]any  `json:"resource_bundle,omitempty"`
	TaskGroupID    *int            `json:"task_group_id,omitempty"`
}

func toTaskJSON(t PipelineTask) taskJSON {
	params := make([]taskParamJSON, len(t.Parameters))

	for i, p := range t.Parameters {
		params[i] = taskParamJSON(p)
	}

	return taskJSON{
		TaskID:         t.TaskID,
		PipelineID:     t.PipelineID,
		VersionID:      t.VersionID,
		Name:           t.Name,
		Parameters:     params,
		Inputs:         t.Inputs,
		Source:         t.Source,
		ResourceBundle: t.ResourceBundle,
		TaskGroupID:    t.TaskGroupID,
	}
}

// RenderTask routes a single task to JSON or human output.
func RenderTask(format outputformat.OutputFormat, t PipelineTask) error {
	if format == outputformat.OutputFormatJSON {
		return printTaskJSON(t)
	}

	printTaskHuman(t)

	return nil
}

// printTaskJSON marshals a task as indented JSON using CLI-vocabulary keys.
func printTaskJSON(t PipelineTask) error {
	data, err := json.MarshalIndent(toTaskJSON(t), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// printTaskHuman renders a single task in a human-friendly tabwriter form.
func printTaskHuman(t PipelineTask) {
	scope := "draft"
	versionDisplay := emptyValuePlaceholder

	if t.VersionID != nil {
		scope = "locked"
		versionDisplay = strconv.Itoa(*t.VersionID)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Task ID:\t%d\n", t.TaskID)
	fmt.Fprintf(w, "Pipeline ID:\t%s\n", t.PipelineID)
	fmt.Fprintf(w, "Scope:\t%s\n", scope)
	fmt.Fprintf(w, "Version:\t%s\n", versionDisplay)
	fmt.Fprintf(w, "Name:\t%s\n", t.Name)

	w.Flush()

	if len(t.Parameters) > 0 {
		fmt.Println()
		fmt.Println(tui.BaseTextStyle.Render("Parameters:"))

		for _, p := range t.Parameters {
			ann := emptyValuePlaceholder

			if p.Annotation != nil {
				ann = *p.Annotation
			}

			fmt.Printf("  %s: %s\n", p.Name, ann)
		}
	}

	if t.Inputs != nil {
		fmt.Println()
		fmt.Println(tui.BaseTextStyle.Render("Inputs:"))

		data, err := json.MarshalIndent(t.Inputs, "  ", "  ")
		if err == nil {
			fmt.Println("  " + strings.TrimPrefix(string(data), "  "))
		}
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Source:"))
	fmt.Println(t.Source)
}
