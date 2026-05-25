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

// version_output.go contains rendering helpers shared by the
// `dr pipelines version` verbs.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

// versionJSON is the CLI-facing DTO used for `--output-format json`.
type versionJSON struct {
	Version        int            `json:"version"`
	Status         string         `json:"status"`
	TaskNames      []string       `json:"task_names,omitempty"`
	PythonVersion  string         `json:"python_version,omitempty"`
	ResourceBundle map[string]any `json:"resource_bundle,omitempty"`
	ErrorDetail    string         `json:"error_detail,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

func toVersionJSON(v PipelineVersion) versionJSON {
	return versionJSON{
		Version:        v.Version,
		Status:         v.Status,
		TaskNames:      v.TaskNames,
		PythonVersion:  v.PythonVersion,
		ResourceBundle: v.ResourceBundle,
		ErrorDetail:    v.ErrorDetail,
		CreatedAt:      v.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// RenderVersion routes a single version to JSON or human output.
func RenderVersion(format OutputFormat, v PipelineVersion) error {
	if format == OutputFormatJSON {
		return PrintVersionJSON(v)
	}

	PrintVersionHuman(v)

	return nil
}

// RenderVersions routes a list of versions to JSON or human output.
func RenderVersions(format OutputFormat, items []PipelineVersion) error {
	if format == OutputFormatJSON {
		return PrintVersionListJSON(items)
	}

	PrintVersionListHuman(items)

	return nil
}

// PrintVersionJSON marshals a single version as indented JSON through the DTO.
func PrintVersionJSON(v PipelineVersion) error {
	data, err := json.MarshalIndent(toVersionJSON(v), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintVersionHuman renders the key facts about a single version.
func PrintVersionHuman(v PipelineVersion) {
	tasks := emptyValuePlaceholder
	if len(v.TaskNames) > 0 {
		tasks = strings.Join(v.TaskNames, ", ")
	}

	python := v.PythonVersion
	if python == "" {
		python = emptyValuePlaceholder
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Version:\tv%s\n", strconv.Itoa(v.Version))
	fmt.Fprintf(w, "Status:\t%s\n", v.Status)
	fmt.Fprintf(w, "Python Version:\t%s\n", python)
	fmt.Fprintf(w, "Tasks:\t%s\n", tasks)

	if v.ErrorDetail != "" {
		fmt.Fprintf(w, "Error:\t%s\n", v.ErrorDetail)
	}

	fmt.Fprintf(w, "Created:\t%s\n", v.CreatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

// PrintVersionListJSON marshals a list of versions as indented JSON through the DTO.
func PrintVersionListJSON(items []PipelineVersion) error {
	view := make([]versionJSON, len(items))

	for i, v := range items {
		view[i] = toVersionJSON(v)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintVersionListHuman renders a lipgloss table summary of versions.
func PrintVersionListHuman(items []PipelineVersion) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No versions found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"VERSION", "STATUS", "PYTHON", "CREATED", "TASKS"}

	createdCol := slices.Index(headers, "CREATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == createdCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, v := range items {
		tasks := emptyValuePlaceholder
		if len(v.TaskNames) > 0 {
			tasks = strings.Join(v.TaskNames, ", ")
		}

		python := v.PythonVersion
		if python == "" {
			python = emptyValuePlaceholder
		}

		t.Row(
			"v"+strconv.Itoa(v.Version),
			v.Status,
			python,
			v.CreatedAt.UTC().Format(timestampFormat),
			tasks,
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}
