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

package wizard

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// envTable is the per-key screen: one row per `.env` variable, showing what
// the classifier made of it and what that means for the file. The classifier
// is good but not right every time, so every row is overridable, which is the
// whole reason this is a table rather than one yes-or-no question.
//
// It renders the plan for the placeholder block that ships today. The credential
// import replaces the plan column's wording, not this component.
type envTable struct {
	rows []EnvVar
	// grid owns the cursor, the scrolling window and the rendering, so this
	// screen looks like the pickers rather than like its own invention.
	grid rowTable
}

// envKindCycle is the order the arrows walk. It starts where the classifier
// put the row and comes back to it, so a user who taps past their answer can
// get back without leaving the screen.
var envKindCycle = []EnvKind{EnvConfig, EnvSecret, EnvLocal}

func newEnvTable(rows []EnvVar, width, height int) envTable {
	t := envTable{rows: append([]EnvVar(nil), rows...)}
	t.grid = newRowTable([]string{"VARIABLE", "CLASSIFIED AS", "WRITTEN AS"}, t.cells(), false, width, height)

	return t
}

// cells projects the classified rows into what the table shows.
func (t envTable) cells() []tableRow {
	out := make([]tableRow, 0, len(t.rows))
	for _, row := range t.rows {
		out = append(out, tableRow{cells: []string{row.Name, row.Kind.String(), plan(row.Kind)}})
	}

	return out
}

// update moves the cursor or changes the row under it, reporting whether the
// key belonged to the table.
func (t *envTable) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case " ", "space", "tab", "right", "l":
		t.cycle(1)

		return true
	case "left", "h":
		t.cycle(-1)

		return true
	}

	return t.grid.update(msg)
}

// cycle moves the row under the cursor to the next or previous plan, and
// records that the user chose it so the reason stops crediting the
// classifier.
//
// The rows are copied first. Bubble Tea hands each Update a copy of the
// model, but a slice field in that copy still points at the original's
// backing array, so writing through it would reach back into the state the
// caller kept.
func (t *envTable) cycle(delta int) {
	if len(t.rows) == 0 {
		return
	}

	t.rows = append([]EnvVar(nil), t.rows...)
	row := &t.rows[t.grid.cursor()]

	for i, kind := range envKindCycle {
		if kind != row.Kind {
			continue
		}

		next := (i + delta + len(envKindCycle)) % len(envKindCycle)
		row.Kind = envKindCycle[next]
		row.Reason = "you chose this"

		break
	}

	t.grid.setRows(t.cells())
}

// vars returns what the file will say about each key.
func (t envTable) vars() []manifest.EnvVar {
	out := make([]manifest.EnvVar, 0, len(t.rows))

	for _, row := range t.rows {
		if row.Kind == EnvLocal {
			// Local-only keys are not listed at all: deploying them would be
			// wrong, not merely unnecessary.
			continue
		}

		out = append(out, manifest.EnvVar{
			Name:   row.Name,
			Value:  row.Value,
			Secret: row.Kind == EnvSecret,
		})
	}

	return out
}

func (t envTable) view(width int) string {
	return t.grid.view(width)
}

// note explains the row under the cursor, so an overridable verdict can be
// judged before it is overridden.
func (t envTable) note() string {
	if len(t.rows) == 0 {
		return ""
	}

	row := t.rows[t.grid.cursor()]

	return row.Name + " — " + row.Reason
}

// plan is what listing this row does to the file, in the file's own terms.
// Kept to two words: the note under the table has room to say more, the
// column does not.
func plan(kind EnvKind) string {
	switch kind {
	case EnvSecret:
		return "credential reference"
	case EnvLocal:
		return "omitted"
	case EnvConfig:
		return "literal value"
	}

	return "literal value"
}

// resize refits the grid to a new terminal size, keeping the cursor and every
// verdict the user has changed.
func (t *envTable) resize(width, height int) {
	t.grid.resize(width, height)
}
