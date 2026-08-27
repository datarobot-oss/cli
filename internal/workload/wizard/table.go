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
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

// How tall a table may get. It takes what the terminal has, up to a limit
// past which a list stops being scannable, and never shrinks below the point
// where scrolling is all you can see.
const (
	maxTableHeight = 20
	minTableHeight = 5

	// chromeRows is what sits above and below a table: the question, the
	// status line, the note and the legend, with their blank lines.
	chromeRows = 10
)

// tableHeight fits the table to the terminal.
func tableHeight(terminalHeight int) int {
	if terminalHeight <= 0 {
		return maxTableHeight
	}

	return min(max(terminalHeight-chromeRows, minTableHeight), maxTableHeight)
}

// maxColumnWidth stops one outlying value from dictating the layout. Some
// execution environments are named with a whole image URI; without a cap that
// one row pushes every other column off the screen.
const maxColumnWidth = 56

// tableRow is one row: the cells as shown, plus whatever the caller needs
// back when the row is chosen.
type tableRow struct {
	cells []string
	value any
}

// rowTable is the wizard's list widget. bubbles/table does the work: cursor,
// scrolling viewport, rendering. This adds the two things it has no opinion
// about, namely type-to-filter and carrying a value alongside each row, and
// keeps every screen that shows a list looking the same.
type rowTable struct {
	model   table.Model
	all     []tableRow
	visible []tableRow
	// headers is kept so the table can be refitted to a new terminal width,
	// which means recomputing the columns it was built with.
	headers []string

	// maxHeight is what the terminal allows; the table itself shrinks to fit
	// however many rows the filter leaves.
	maxHeight int

	filter     string
	filterable bool
}

func newRowTable(headers []string, rows []tableRow, filterable bool, width, height int) rowTable {
	t := rowTable{all: rows, headers: headers, filterable: filterable, maxHeight: tableHeight(height)}
	t.model = table.New(
		table.WithColumns(columnsFor(headers, rows, width)),
		table.WithHeight(t.maxHeight),
		table.WithFocused(true),
		table.WithStyles(tableStyles()),
	)

	t.apply()

	return t
}

// resize refits the table to a terminal it did not know about when it was
// built, keeping the filter and the row under the cursor. Columns and height
// are decided at construction, so without this a table built before the first
// size message is stuck at the fallback for the rest of the run.
func (t *rowTable) resize(width, height int) {
	if len(t.all) == 0 {
		return
	}

	cursor := t.model.Cursor()

	t.maxHeight = tableHeight(height)
	t.model.SetColumns(columnsFor(t.headers, t.all, width))
	t.syncRows()
	t.model.SetCursor(cursor)
	t.syncRows()
	t.fit()
}

// tableStyles dresses bubbles/table in the shared palette, so it matches the
// static tables the rest of the CLI prints.
func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = labelStyle.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(tui.BorderColor).
		BorderBottom(true).
		Bold(true).
		Padding(0, 1)
	// Cells carry no colour of their own. bubbles/table renders each cell and
	// then wraps the cursor row in Selected; a cell that set its own foreground
	// would emit a reset that swallows the selected row's colour, so the colour
	// is left off here and the selected row is the only thing coloured.
	s.Cell = lipgloss.NewStyle().Padding(0, 1)
	// No padding on Selected: it wraps the already-padded row, so padding again
	// would shift the highlighted row a column out of line with the rest.
	s.Selected = selectedStyle

	return s
}

// columnsFor sizes each column to its widest cell, then shares out whatever
// the terminal has left so the last column absorbs the slack rather than the
// table overflowing.
func columnsFor(headers []string, rows []tableRow, width int) []table.Column {
	if len(headers) == 0 {
		return nil
	}

	// A narrow first column carries the cursor marker.
	headers = append([]string{" "}, headers...)

	columns := make([]table.Column, len(headers))

	for i, header := range headers {
		columns[i] = table.Column{Title: header, Width: len(header)}
	}

	columns[0].Width = 2

	for _, row := range rows {
		for i, cell := range row.cells {
			if i+1 < len(columns) {
				columns[i+1].Width = max(columns[i+1].Width, min(len(cell), maxColumnWidth))
			}
		}
	}

	// Two of padding per column, which is what the cell style adds.
	const padding = 2

	total := 0
	for _, c := range columns {
		total += c.Width + padding
	}

	// A table narrower than the terminal is left narrow: stretching it to the
	// edge only puts a corridor of blank space between the columns.
	if over := total - contentWidth(width); over > 0 {
		// Take it out of the widest column: that is the one carrying names
		// long enough to have caused the overflow.
		widest := 0
		for i, c := range columns {
			if c.Width > columns[widest].Width {
				widest = i
			}
		}

		columns[widest].Width = max(columns[widest].Width-over, len(columns[widest].Title))
	}

	return columns
}

// selected is the row under the cursor, nil when the filter admits nothing.
func (t rowTable) selected() *tableRow {
	cursor := t.model.Cursor()
	if cursor < 0 || cursor >= len(t.visible) {
		return nil
	}

	return &t.visible[cursor]
}

// update handles a keystroke, reporting whether the table consumed it.
//
// There is no filter mode. Typing filters, the arrows move, enter selects,
// and every one of those means the same thing at every moment. The earlier
// design had a mode entered with "/", inside which enter meant "accept the
// filter" while the legend still said "select": two meanings for one key,
// and no way to tell which one was live.
func (t *rowTable) update(msg tea.KeyMsg) bool {
	if !t.filterable {
		return t.moveCursor(msg)
	}

	switch {
	case msg.Type == tea.KeyBackspace:
		if t.filter == "" {
			return false
		}

		// One keystroke removes one character, not one byte: a workload named
		// in anything but ASCII would otherwise take several backspaces per
		// letter and leave a broken encoding in between.
		_, size := utf8.DecodeLastRuneInString(t.filter)
		t.filter = t.filter[:len(t.filter)-size]

		t.apply()

		return true

	case msg.String() == "/" && t.filter == "":
		// Muscle memory: in most pickers "/" starts a filter. Here typing
		// already does, so it is swallowed rather than searched for. Once
		// something is typed it is a character like any other, which matters
		// for execution environments named after image URIs.
		return true

	case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
		t.filter += string(msg.Runes)

		t.apply()

		return true

	case msg.Type == tea.KeySpace:
		// Space arrives as its own key type, never inside KeyRunes, and the
		// table underneath binds it to page-down — so without this case a
		// name like "Py 3.12" could not be typed. In a filterable table every
		// printable character is filter input ("f" and "b" already are), and
		// paging keeps pgup/pgdown.
		//
		// As a first keystroke it is swallowed like "/" above: a filter of
		// one space is invisible on screen, and the esc that then seems to
		// do nothing would be spent clearing it instead of going back.
		if t.filter != "" {
			t.filter += " "

			t.apply()
		}

		return true
	}

	return t.moveCursor(msg)
}

// moveCursor hands the key to bubbles/table and reports whether it moved.
func (t *rowTable) moveCursor(msg tea.KeyMsg) bool {
	before := t.model.Cursor()
	t.model, _ = t.model.Update(msg)

	if t.model.Cursor() == before {
		return false
	}

	// The marker rides in a cell, so the rows are rebuilt when it moves.
	t.syncRows()

	return true
}

// clearFilter drops the query, reporting whether there was one. Esc clears
// before it goes back, so a filter typed by accident is one key to undo.
func (t *rowTable) clearFilter() bool {
	if t.filter == "" {
		return false
	}

	t.filter = ""

	t.apply()

	return true
}

// headerRows is what bubbles/table spends on the header and its rule.
// SetHeight counts them, so a height of n shows n-2 rows.
const headerRows = 2

// fit keeps the table as tall as its contents, up to what the terminal
// allows. Without this a short list sits in a block of blank rows, and a
// filtered one collapses to a single line.
func (t *rowTable) fit() {
	t.model.SetHeight(max(min(t.maxHeight, len(t.visible)), 1) + headerRows)
}

// apply narrows the rows to the filter and hands them to the table.
func (t *rowTable) apply() {
	t.visible = t.all

	if t.filter != "" {
		query := strings.ToLower(t.filter)
		t.visible = make([]tableRow, 0, len(t.all))

		for _, row := range t.all {
			if strings.Contains(strings.ToLower(strings.Join(row.cells, " ")), query) {
				t.visible = append(t.visible, row)
			}
		}
	}

	// Rows before cursor: bubbles/table clamps the cursor to the row count,
	// so setting it against an empty table pins it at -1 and nothing is ever
	// selected. Then the markers are laid on again, now that there is a
	// cursor for them to follow.
	t.syncRows()
	t.model.SetCursor(0)
	t.syncRows()
	t.fit()
}

// syncRows hands the visible rows to the table, each prefixed with the marker
// column so the row under the cursor is unmistakable rather than merely a
// different colour.
func (t *rowTable) syncRows() {
	rows := make([]table.Row, 0, len(t.visible))

	for i, row := range t.visible {
		marker := "  "
		if i == t.model.Cursor() {
			marker = " ❯"
		}

		rows = append(rows, table.Row(append([]string{marker}, row.cells...)))
	}

	t.model.SetRows(rows)
}

// setRows replaces the contents, keeping the filter and the styling.
func (t *rowTable) setRows(rows []tableRow) {
	cursor := t.model.Cursor()
	t.all = rows

	t.apply()
	t.model.SetCursor(cursor)
	t.syncRows()
}

// selectValue moves the cursor to the first row whose value satisfies match,
// so a screen rebuilt on the way back opens on the answer given there rather
// than on whatever happens to be first.
func (t *rowTable) selectValue(match func(any) bool) {
	for i, row := range t.visible {
		if !match(row.value) {
			continue
		}

		t.model.SetCursor(i)
		t.syncRows()

		return
	}
}

// cursor is the index into the visible rows.
func (t rowTable) cursor() int {
	return t.model.Cursor()
}

func (t rowTable) view(width int) string {
	view := lipgloss.NewStyle().MarginLeft(2).Render(t.model.View())

	// The filter sits above the rows it narrows, the way a search box does;
	// the position counter sits below, the way a scrollbar does.
	if filter := t.filterLine(); filter != "" {
		view = filter + "\n\n" + view
	}

	if position := t.positionLine(width); position != "" {
		view += "\n" + position
	}

	return view
}

// filterLine shows the query, with the caret because typing always goes here.
func (t rowTable) filterLine() string {
	if t.filter == "" {
		return ""
	}

	return "  " + filterStyle.Render("filter: "+t.filter+"▌")
}

// positionLine says where the cursor is in a list too long to show at once,
// and reports a filter that matched nothing.
func (t rowTable) positionLine(width int) string {
	switch {
	case len(t.visible) == 0 && t.filter != "":
		return noteStyle.Width(contentWidth(width)).Render("nothing matches")
	case len(t.visible) > t.maxHeight:
		return noteStyle.Width(contentWidth(width)).
			Render(fmt.Sprintf("%d of %d", t.model.Cursor()+1, len(t.visible)))
	default:
		return ""
	}
}
