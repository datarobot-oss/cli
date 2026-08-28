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

package doctor

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/datarobot/cli/tui"
)

// WriteText renders a report in human-readable form: a header (project dir
// and linked artifact or "not linked"), a CHECK/STATUS/DETAIL table with one
// row per check in runner order, remedies for non-OK rows, and a summary line
// with per-status counts plus the overall verdict.
func WriteText(w io.Writer, report Report) error {
	artifact := report.linkedArtifact()
	if artifact == "" {
		artifact = "not linked"
	}

	fmt.Fprintf(w, "Doctor report for %s — artifact: %s\n\n", report.ProjectDir, artifact)

	if err := writeChecksTable(w, report); err != nil {
		return err
	}

	writeRemedies(w, report)

	writeActions(w, report)

	writeSummary(w, report)

	return nil
}

// writeChecksTable renders the per-check table using the repo-standard
// lipgloss table styling.
func writeChecksTable(w io.Writer, report Report) error {
	statusByRow := make(map[int]Status, len(report.Checks))

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	statusCol := 1

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == statusCol {
				switch statusByRow[row] {
				case StatusOK:
					return tui.SuccessStyle.Padding(0, 1)
				case StatusWARN:
					return tui.WarnStyle.Padding(0, 1)
				case StatusFAIL:
					return tui.ErrorStyle.Padding(0, 1)
				case StatusSKIP:
					return tui.DimStyle.Padding(0, 1)
				}
			}

			return cellStyle
		}).
		Headers("CHECK", "STATUS", "DETAIL")

	for i, res := range report.Checks {
		statusByRow[i] = res.Status

		t.Row(res.CheckID, string(res.Status), renderDetail(res))
	}

	_, err := fmt.Fprintln(w, t.Render())

	return err
}

// renderDetail builds the DETAIL cell: the summary plus any structured
// details (e.g. "path: /abs/file") on their own lines, keys sorted for
// deterministic output.
func renderDetail(res Result) string {
	parts := make([]string, 0, len(res.Details)+1)

	parts = append(parts, res.Summary)

	keys := make([]string, 0, len(res.Details))

	for k := range res.Details {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	for _, k := range keys {
		parts = append(parts, k+": "+res.Details[k])
	}

	return strings.Join(parts, "\n")
}

// writeRemedies prints the remedy for each non-OK check that carries one.
func writeRemedies(w io.Writer, report Report) {
	remedies := make([]string, 0, len(report.Checks))

	for _, res := range report.Checks {
		if res.Status == StatusOK || res.Remedy == "" {
			continue
		}

		remedies = append(remedies, fmt.Sprintf("  %s: %s", res.CheckID, res.Remedy))
	}

	if len(remedies) == 0 {
		return
	}

	fmt.Fprintln(w, "\nRemedies")

	for _, r := range remedies {
		fmt.Fprintln(w, r)
	}
}

// writeActions prints the per-repair outcomes of a repair run (--fix /
// --relink); read-only runs (Actions nil) print nothing. When every repair
// reported not-needed, the section says so explicitly: a --fix on a healthy
// project is a no-op and the output must state that unambiguously.
func writeActions(w io.Writer, report Report) {
	if report.Actions == nil {
		return
	}

	actions := *report.Actions

	fmt.Fprintln(w, "\nRepairs")

	if len(actions) == 0 {
		fmt.Fprintln(w, "  nothing to fix: no repairs needed")

		return
	}

	allNotNeeded := true

	for _, action := range actions {
		if action.Status != ActionNotNeeded {
			allNotNeeded = false
		}

		line := fmt.Sprintf("  %s: %s", action.ID, action.Status)

		if action.Reason != "" {
			line += " — " + action.Reason
		}

		fmt.Fprintln(w, line)
	}

	if allNotNeeded {
		fmt.Fprintln(w, "  nothing to fix: no repairs needed")
	}
}

// writeSummary prints the per-status counts and the overall verdict.
func writeSummary(w io.Writer, report Report) {
	counts := report.Counts()

	fmt.Fprintf(w, "\nSummary: %d ok, %d warn, %d fail, %d skip — verdict: %s\n",
		counts.OK, counts.WARN, counts.FAIL, counts.SKIP, report.OverallStatus())
}
