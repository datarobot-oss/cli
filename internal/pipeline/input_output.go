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

// input_output.go centralises the human/JSON output rendering used by the
// input verbs so each verb file stays focused on flag wiring.
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

// inputJSON is the CLI-facing DTO used for `--output-format json`.
type inputJSON struct {
	InputID    string          `json:"input_id"`
	PipelineID string          `json:"pipeline_id"`
	Scope      string          `json:"scope"`
	Version    string          `json:"version"`
	State      string          `json:"state"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

func toInputJSON(input Input) inputJSON {
	scope := "draft"
	version := emptyValuePlaceholder

	if input.VersionID != nil {
		scope = "locked"
		version = "v" + strconv.Itoa(*input.VersionID)
	}

	payloadBytes, _ := json.Marshal(input.Payload)

	return inputJSON{
		InputID:    input.InputID,
		PipelineID: input.PipelineID,
		Scope:      scope,
		Version:    version,
		State:      string(input.State),
		Payload:    payloadBytes,
		CreatedAt:  input.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  input.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RenderInput routes a single input to JSON or human output.
func RenderInput(format OutputFormat, input Input) error {
	if format == OutputFormatJSON {
		return PrintInputJSON(input)
	}

	PrintInputHuman(input)

	return nil
}

// RenderInputs routes a list of inputs to JSON or human output.
func RenderInputs(format OutputFormat, inputs []Input) error {
	if format == OutputFormatJSON {
		return PrintInputListJSON(inputs)
	}

	PrintInputListHuman(inputs)

	return nil
}

// PrintInputJSON marshals an input record as indented JSON through the DTO.
func PrintInputJSON(input Input) error {
	data, err := json.MarshalIndent(toInputJSON(input), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintInputHuman renders the key facts about a single input record.
func PrintInputHuman(input Input) {
	scope := "draft"
	versionDisplay := emptyValuePlaceholder

	if input.VersionID != nil {
		scope = "locked"
		versionDisplay = "v" + strconv.Itoa(*input.VersionID)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Input ID:\t%s\n", input.InputID)
	fmt.Fprintf(w, "Pipeline ID:\t%s\n", input.PipelineID)
	fmt.Fprintf(w, "Scope:\t%s\n", scope)
	fmt.Fprintf(w, "Version:\t%s\n", versionDisplay)
	fmt.Fprintf(w, "State:\t%s\n", string(input.State))
	fmt.Fprintf(w, "Created:\t%s\n", input.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", input.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()

	payload, err := json.MarshalIndent(input.Payload, "", "  ")
	if err != nil {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Payload:"))
	fmt.Println(string(payload))
}

// PrintInputListJSON marshals a list of inputs as indented JSON through the DTO.
func PrintInputListJSON(inputs []Input) error {
	view := make([]inputJSON, len(inputs))

	for i, in := range inputs {
		view[i] = toInputJSON(in)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintInputListHuman renders a lipgloss table summary of inputs.
func PrintInputListHuman(inputs []Input) {
	if len(inputs) == 0 {
		fmt.Println(tui.DimStyle.Render("No inputs found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"INPUT ID", "SCOPE", "VERSION", "STATE", "UPDATED"}

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

	for _, in := range inputs {
		scope := "draft"
		ver := emptyValuePlaceholder

		if in.VersionID != nil {
			scope = "locked"
			ver = "v" + strconv.Itoa(*in.VersionID)
		}

		t.Row(in.InputID, scope, ver, string(in.State), in.UpdatedAt.UTC().Format(timestampFormat))
	}

	fmt.Fprintln(os.Stdout, t.Render())
}
