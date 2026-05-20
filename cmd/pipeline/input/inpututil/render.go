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

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// PrintInputJSON marshals an input record as indented JSON.
func PrintInputJSON(input pipeline.Input) error {
	data, err := json.MarshalIndent(input, "", "  ")
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
	fmt.Println(tui.DimStyle.Render("Created:     " + input.CreatedAt))
	fmt.Println(tui.DimStyle.Render("Updated:     " + input.UpdatedAt))

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
	data, err := json.MarshalIndent(inputs, "", "  ")
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
			in.InputID, scope, ver, in.State, in.UpdatedAt,
		)
	}

	_ = writer.Flush()
}
