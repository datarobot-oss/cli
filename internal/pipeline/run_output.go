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

// run_output.go holds the rendering helpers shared by the
// `dr pipelines run` verbs.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

// runJSON is the CLI-facing shape used for `--output-format json`. It mirrors
// Run but renames the wire-level fields to the CLI's `run`
// vocabulary (`run_id`, `covalent_run_id`). Decoding still happens
// through Run, which keeps the API wire tags intact.
type runJSON struct {
	RunID         string `json:"run_id"`
	PipelineID    string `json:"pipeline_id"`
	VersionID     *int   `json:"version_id,omitempty"`
	InputID       string `json:"input_id"`
	CovalentRunID string `json:"covalent_run_id,omitempty"`
	TriggeredBy   string `json:"triggered_by"`
	Status        string `json:"status"`
	ErrorDetail   string `json:"error_detail,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func toRunJSON(r Run) runJSON {
	return runJSON{
		RunID:         r.RunID,
		PipelineID:    r.PipelineID,
		VersionID:     r.VersionID,
		InputID:       r.InputID,
		CovalentRunID: r.CovalentDispatchID,
		TriggeredBy:   r.TriggeredBy,
		Status:        r.Status,
		ErrorDetail:   r.ErrorDetail,
		CreatedAt:     r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// runStatusJSON mirrors RunStatus with CLI-vocabulary keys.
type runStatusJSON struct {
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	CovalentRunID string `json:"covalent_run_id,omitempty"`
}

func toRunStatusJSON(s RunStatus) runStatusJSON {
	return runStatusJSON{
		RunID:         s.RunID,
		Status:        s.Status,
		CovalentRunID: s.CovalentDispatchID,
	}
}

// RenderRun routes a single run to JSON or human output.
func RenderRun(format OutputFormat, r Run) error {
	if format == OutputFormatJSON {
		return PrintRunJSON(r)
	}

	PrintRunHuman(r)

	return nil
}

// RenderRuns routes a list of runs to JSON or human output.
func RenderRuns(format OutputFormat, items []Run) error {
	if format == OutputFormatJSON {
		return PrintRunListJSON(items)
	}

	PrintRunListHuman(items)

	return nil
}

// RenderRunStatus routes a run status to JSON or human output.
func RenderRunStatus(format OutputFormat, s RunStatus) error {
	if format == OutputFormatJSON {
		return PrintStatusJSON(s)
	}

	PrintStatusHuman(s)

	return nil
}

// PrintRunJSON marshals a run as indented JSON using CLI-vocabulary keys.
func PrintRunJSON(r Run) error {
	data, err := json.MarshalIndent(toRunJSON(r), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintRunHuman renders a single run in a human-friendly form.
func PrintRunHuman(r Run) {
	scope := "draft"
	versionDisplay := emptyValuePlaceholder

	if r.VersionID != nil {
		scope = "locked"
		versionDisplay = "v" + strconv.Itoa(*r.VersionID)
	}

	covalent := r.CovalentDispatchID
	if covalent == "" {
		covalent = emptyValuePlaceholder
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Run ID:\t%s\n", r.RunID)
	fmt.Fprintf(w, "Pipeline ID:\t%s\n", r.PipelineID)
	fmt.Fprintf(w, "Scope:\t%s\n", scope)
	fmt.Fprintf(w, "Version:\t%s\n", versionDisplay)
	fmt.Fprintf(w, "Input ID:\t%s\n", r.InputID)
	fmt.Fprintf(w, "Status:\t%s\n", r.Status)
	fmt.Fprintf(w, "Triggered By:\t%s\n", r.TriggeredBy)
	fmt.Fprintf(w, "Covalent Run:\t%s\n", covalent)

	if r.ErrorDetail != "" {
		fmt.Fprintf(w, "Error:\t%s\n", r.ErrorDetail)
	}

	fmt.Fprintf(w, "Created:\t%s\n", r.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", r.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

// PrintRunListJSON marshals a list of runs as indented JSON using
// CLI-vocabulary keys.
func PrintRunListJSON(items []Run) error {
	view := make([]runJSON, len(items))

	for i, r := range items {
		view[i] = toRunJSON(r)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintRunListHuman renders a lipgloss table summary of runs.
func PrintRunListHuman(items []Run) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No runs found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"RUN ID", "SCOPE", "VERSION", "STATUS", "TRIGGER", "UPDATED"}

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

	for _, r := range items {
		scope := "draft"
		ver := emptyValuePlaceholder

		if r.VersionID != nil {
			scope = "locked"
			ver = "v" + strconv.Itoa(*r.VersionID)
		}

		t.Row(r.RunID, scope, ver, r.Status, r.TriggeredBy, r.UpdatedAt.UTC().Format(timestampFormat))
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// PrintStatusJSON marshals a lightweight status response as indented JSON
// using CLI-vocabulary keys.
func PrintStatusJSON(s RunStatus) error {
	data, err := json.MarshalIndent(toRunStatusJSON(s), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintStatusHuman renders a lightweight status response.
func PrintStatusHuman(s RunStatus) {
	covalent := s.CovalentDispatchID
	if covalent == "" {
		covalent = emptyValuePlaceholder
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Run ID:\t%s\n", s.RunID)
	fmt.Fprintf(w, "Status:\t%s\n", s.Status)
	fmt.Fprintf(w, "Covalent Run:\t%s\n", covalent)

	w.Flush()
}
