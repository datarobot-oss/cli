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

// Package runutil holds the rendering helpers shared by the
// `dr pipelines run` verbs. Living in a sibling package keeps the
// parent `run` package free of cycles.

package runutil

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// runJSON is the CLI-facing shape used for `--output json`. It mirrors
// pipeline.Run but renames the wire-level fields to the CLI's `run`
// vocabulary (`run_id`, `covalent_run_id`). Decoding still happens
// through pipeline.Run, which keeps the API wire tags intact.
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

func toRunJSON(r pipeline.Run) runJSON {
	return runJSON{
		RunID:         r.RunID,
		PipelineID:    r.PipelineID,
		VersionID:     r.VersionID,
		InputID:       r.InputID,
		CovalentRunID: r.CovalentDispatchID,
		TriggeredBy:   r.TriggeredBy,
		Status:        r.Status,
		ErrorDetail:   r.ErrorDetail,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// runStatusJSON mirrors pipeline.RunStatus with CLI-vocabulary keys.
type runStatusJSON struct {
	RunID         string `json:"run_id"`
	Status        string `json:"status"`
	CovalentRunID string `json:"covalent_run_id,omitempty"`
}

func toRunStatusJSON(s pipeline.RunStatus) runStatusJSON {
	return runStatusJSON{
		RunID:         s.RunID,
		Status:        s.Status,
		CovalentRunID: s.CovalentDispatchID,
	}
}

// PrintRunJSON marshals a run as indented JSON using CLI-vocabulary keys.
func PrintRunJSON(r pipeline.Run) error {
	data, err := json.MarshalIndent(toRunJSON(r), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintRunHuman renders a single run in a human-friendly form.
func PrintRunHuman(r pipeline.Run) {
	scope := "draft"
	versionDisplay := "\u2014"

	if r.VersionID != nil {
		scope = "locked"
		versionDisplay = "v" + strconv.Itoa(*r.VersionID)
	}

	covalent := r.CovalentDispatchID
	if covalent == "" {
		covalent = "\u2014"
	}

	fmt.Println(tui.BaseTextStyle.Render("Run ID:        " + r.RunID))
	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID:   " + r.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Scope:         " + scope))
	fmt.Println(tui.BaseTextStyle.Render("Version:       " + versionDisplay))
	fmt.Println(tui.BaseTextStyle.Render("Input ID:      " + r.InputID))
	fmt.Println(tui.BaseTextStyle.Render("Status:        " + r.Status))
	fmt.Println(tui.BaseTextStyle.Render("Triggered By:  " + r.TriggeredBy))
	fmt.Println(tui.BaseTextStyle.Render("Covalent Run:  " + covalent))

	if r.ErrorDetail != "" {
		fmt.Println(tui.BaseTextStyle.Render("Error:         " + r.ErrorDetail))
	}

	fmt.Println(tui.DimStyle.Render("Created:       " + r.CreatedAt))
	fmt.Println(tui.DimStyle.Render("Updated:       " + r.UpdatedAt))
}

// PrintRunListJSON marshals a list of runs as indented JSON using
// CLI-vocabulary keys.
func PrintRunListJSON(items []pipeline.Run) error {
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

// PrintRunListHuman renders a tabular summary of runs.
func PrintRunListHuman(items []pipeline.Run) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No runs found"))

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "RUN_ID\tSCOPE\tVERSION\tSTATUS\tTRIGGER\tUPDATED")

	for _, r := range items {
		scope := "draft"
		ver := "\u2014"

		if r.VersionID != nil {
			scope = "locked"
			ver = "v" + strconv.Itoa(*r.VersionID)
		}

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.RunID, scope, ver, r.Status, r.TriggeredBy, r.UpdatedAt,
		)
	}

	_ = writer.Flush()
}

// PrintStatusJSON marshals a lightweight status response as indented JSON
// using CLI-vocabulary keys.
func PrintStatusJSON(s pipeline.RunStatus) error {
	data, err := json.MarshalIndent(toRunStatusJSON(s), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintStatusHuman renders a lightweight status response.
func PrintStatusHuman(s pipeline.RunStatus) {
	covalent := s.CovalentDispatchID
	if covalent == "" {
		covalent = "\u2014"
	}

	fmt.Println(tui.BaseTextStyle.Render("Run ID:        " + s.RunID))
	fmt.Println(tui.BaseTextStyle.Render("Status:        " + s.Status))
	fmt.Println(tui.BaseTextStyle.Render("Covalent Run:  " + covalent))
}
