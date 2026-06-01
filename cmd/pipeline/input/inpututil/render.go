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

// render.go centralises the human/JSON output rendering used by the input
// verbs so each verb file stays focused on flag wiring.

package inpututil

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// inputJSON is the CLI-facing JSON shape, remapping wire-level "id" to "input_id".
type inputJSON struct {
	InputID    string         `json:"input_id"`
	PipelineID string         `json:"pipeline_id"`
	VersionID  *int           `json:"version_id,omitempty"`
	IsDraft    bool           `json:"is_draft"`
	State      string         `json:"state"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

func toInputJSON(in pipeline.Input) inputJSON {
	return inputJSON{
		InputID:    in.InputID,
		PipelineID: in.PipelineID,
		VersionID:  in.VersionID,
		IsDraft:    in.IsDraft,
		State:      string(in.State),
		Payload:    in.Payload,
		CreatedAt:  in.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  in.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// PrintInputJSON marshals an input record as indented JSON.
func PrintInputJSON(input pipeline.Input) error {
	data, err := json.MarshalIndent(toInputJSON(input), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintInputHuman renders the key facts about a single input record.
func PrintInputHuman(input pipeline.Input) {
	scope := "draft"
	versionDisplay := "\u2014"

	if input.VersionID != nil {
		scope = "locked"
		versionDisplay = "v" + strconv.Itoa(*input.VersionID)
	}

	fmt.Println(tui.BaseTextStyle.Render("Input ID:    " + input.InputID))
	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID: " + input.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Scope:       " + scope))
	fmt.Println(tui.BaseTextStyle.Render("Version:     " + versionDisplay))
	fmt.Println(tui.BaseTextStyle.Render("State:       " + string(input.State)))
	fmt.Println(tui.DimStyle.Render("Created:     " + input.CreatedAt.UTC().Format(time.RFC3339)))
	fmt.Println(tui.DimStyle.Render("Updated:     " + input.UpdatedAt.UTC().Format(time.RFC3339)))

	payload, err := json.MarshalIndent(input.Payload, "", "  ")
	if err != nil {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Payload:"))
	fmt.Println(string(payload))
}

// PrintInputListJSON marshals a list of inputs as indented JSON.
func PrintInputListJSON(inputs []pipeline.Input) error {
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

// PrintInputListHuman renders a tabular summary of inputs.
func PrintInputListHuman(inputs []pipeline.Input) {
	if len(inputs) == 0 {
		fmt.Println(tui.DimStyle.Render("No inputs found"))

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "INPUT_ID\tSCOPE\tVERSION\tSTATE\tUPDATED")

	for _, in := range inputs {
		scope := "draft"
		ver := "\u2014"

		if in.VersionID != nil {
			scope = "locked"
			ver = "v" + strconv.Itoa(*in.VersionID)
		}

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			in.InputID, scope, ver, in.State, in.UpdatedAt.UTC().Format(time.RFC3339),
		)
	}

	_ = writer.Flush()
}
