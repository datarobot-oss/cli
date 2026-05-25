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

// pipeline_output.go holds the rendering helpers shared by the top-level
// `dr pipelines` verbs (list, get, create, update, lock).
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

const (
	timestampFormat       = "2006-01-02 15:04 UTC"
	emptyValuePlaceholder = "—"
)

// RenderPipeline routes a single pipeline to JSON or human output.
func RenderPipeline(format OutputFormat, p Pipeline) error {
	if format == OutputFormatJSON {
		return printPipelineJSON(p)
	}

	printPipelineHuman(p)

	return nil
}

// RenderPipelines routes a pipeline list to JSON or human output.
func RenderPipelines(format OutputFormat, page DataPage[ListItem]) error {
	if format == OutputFormatJSON {
		return printPipelinesJSON(page)
	}

	printPipelinesHuman(page)

	return nil
}

// RenderCreateResponse routes a CreateResponse to JSON or human output.
func RenderCreateResponse(format OutputFormat, result CreateResponse) error {
	if format == OutputFormatJSON {
		return printCreateResponseJSON(result)
	}

	printCreateResponseHuman(result)

	return nil
}

func printPipelineJSON(p Pipeline) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printPipelinesJSON(page DataPage[ListItem]) error {
	data, err := json.MarshalIndent(page.Data, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printCreateResponseJSON(result CreateResponse) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printPipelinesHuman(page DataPage[ListItem]) {
	if len(page.Data) == 0 {
		fmt.Println(tui.DimStyle.Render("No pipelines found."))

		return
	}

	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Showing %d of %d", len(page.Data), page.TotalCount)))
	fmt.Println()

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"ID", "NAME", "MODE", "ACTIVE", "VERSION", "UPDATED"}

	updatedCol := slices.Index(headers, "UPDATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == updatedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, item := range page.Data {
		latest := emptyValuePlaceholder
		if item.LatestVersion != nil {
			latest = "v" + strconv.Itoa(*item.LatestVersion)
		}

		updated := item.UpdatedAt.UTC().Format(timestampFormat)
		active := strconv.FormatBool(item.IsActive)

		t.Row(item.PipelineID, item.Name, item.Mode, active, latest, updated)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

func printPipelineHuman(p Pipeline) {
	description := emptyValuePlaceholder
	if p.Description != "" {
		description = p.Description
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", p.PipelineID)
	fmt.Fprintf(w, "Name:\t%s\n", p.Name)
	fmt.Fprintf(w, "Description:\t%s\n", description)
	fmt.Fprintf(w, "Mode:\t%s\n", p.Mode)
	fmt.Fprintf(w, "Active:\t%t\n", p.IsActive)
	fmt.Fprintf(w, "Created:\t%s\n", p.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", p.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()

	if len(p.Versions) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Versions (%d):", len(p.Versions))))

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

	for _, ver := range p.Versions {
		tasks := emptyValuePlaceholder
		if len(ver.TaskNames) > 0 {
			tasks = strings.Join(ver.TaskNames, ", ")
		}

		python := ver.PythonVersion
		if python == "" {
			python = emptyValuePlaceholder
		}

		t.Row(
			"v"+strconv.Itoa(ver.Version),
			ver.Status,
			python,
			ver.CreatedAt.UTC().Format(timestampFormat),
			tasks,
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())

	for _, ver := range p.Versions {
		if ver.ErrorDetail == "" {
			continue
		}

		fmt.Println(tui.DimStyle.Render(fmt.Sprintf("  v%d error: %s", ver.Version, ver.ErrorDetail)))
	}
}

func printCreateResponseHuman(result CreateResponse) {
	tasks := emptyValuePlaceholder
	if len(result.TaskNames) > 0 {
		tasks = strings.Join(result.TaskNames, ", ")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Pipeline ID:\t%s\n", result.PipelineID)
	fmt.Fprintf(w, "Name:\t%s\n", result.Name)
	fmt.Fprintf(w, "Version:\t%d\n", result.Version)
	fmt.Fprintf(w, "Status:\t%s\n", result.Status)
	fmt.Fprintf(w, "Mode:\t%s\n", result.Mode)
	fmt.Fprintf(w, "Tasks:\t%s\n", tasks)
	fmt.Fprintf(w, "Created:\t%s\n", result.CreatedAt.UTC().Format(timestampFormat))

	w.Flush()
}
