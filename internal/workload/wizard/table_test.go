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
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRows builds n rows named row-0, row-1, and so on.
func fakeRows(n int) []tableRow {
	rows := make([]tableRow, 0, n)
	for i := range n {
		rows = append(rows, tableRow{cells: []string{fmt.Sprintf("row-%d", i), "detail"}, value: i})
	}

	return rows
}

// typeInto sends each rune of text to the table.
func typeFilter(t *rowTable, text string) {
	for _, r := range text {
		t.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// visibleRowCount counts the data rows the table drew, so the assertions are
// about what the user sees rather than about internal state. The rendered
// shape is an optional filter line, the header, its rule, the rows, then an
// optional position line: anchoring on the rule keeps this honest as the
// surrounding furniture moves.
func visibleRowCount(view string) int {
	lines := strings.Split(view, "\n")

	rule := -1

	for i, line := range lines {
		if strings.Contains(line, "─") {
			rule = i

			break
		}
	}

	if rule < 0 {
		return 0
	}

	count := 0

	for _, line := range lines[rule+1:] {
		trimmed := strings.TrimSpace(stripANSI(line))

		switch {
		case trimmed == "":
			continue
		case isStatusLine(trimmed):
			return count
		default:
			count++
		}
	}

	return count
}

func isStatusLine(line string) bool {
	if strings.HasPrefix(line, "nothing matches") {
		return true
	}

	// The position counter, "3 of 200".
	return regexp.MustCompile(`^\d+ of \d+`).MatchString(line)
}

// stripANSI removes styling so the assertions read the text, not the colour.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

// A table is as tall as its contents, so a short list does not sit in a block
// of blank rows and a filtered one does not collapse to a single line.
func TestRowTable_HeightFollowsContents(t *testing.T) {
	tests := map[string]struct {
		rows, terminal, want int
	}{
		"short list on a tall terminal":  {5, 44, 5},
		"long list is capped":            {200, 44, maxTableHeight},
		"short terminal shows fewer":     {200, 20, 20 - chromeRows},
		"tiny terminal still shows some": {200, 8, minTableHeight},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			table := newRowTable([]string{"NAME", "DETAIL"}, fakeRows(test.rows), true, 120, test.terminal)

			assert.Equal(t, test.want, visibleRowCount(table.view(120)))
		})
	}
}

// Filtering matches on any cell, case-insensitively, and the table shrinks to
// what is left.
func TestRowTable_Filter(t *testing.T) {
	rows := []tableRow{
		{cells: []string{"[DataRobot] Python 3.11 Drop-In", "a"}, value: 1},
		{cells: []string{"DRUM NIM Sidecar", "b"}, value: 2},
		{cells: []string{"fastdrum-genai", "c"}, value: 3},
		{cells: []string{"ldtest-2", "d"}, value: 4},
	}

	table := newRowTable([]string{"NAME", "DETAIL"}, rows, true, 120, 44)
	require.Equal(t, 4, visibleRowCount(table.view(120)))

	// Matches "Drop-In", "DRUM" and "fastdrum": any cell, either case.
	// There is no filter mode to enter; typing is filtering.
	typeFilter(&table, "dr")
	assert.Equal(t, 3, visibleRowCount(table.view(120)))
	assert.Contains(t, table.view(120), "filter: dr")

	typeFilter(&table, "um")
	assert.Equal(t, 2, visibleRowCount(table.view(120)), "'drum' drops Drop-In")

	// Backspace widens it again.
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, 3, visibleRowCount(table.view(120)))
	assert.Contains(t, table.view(120), "filter: dr")

	// Enter is never the table's: it selects, whatever is typed.
	assert.False(t, table.update(tea.KeyMsg{Type: tea.KeyEnter}))
	assert.Equal(t, 3, visibleRowCount(table.view(120)))

	// clearFilter is what esc reaches for, and it reports whether it did
	// anything so the caller knows to go back instead.
	assert.True(t, table.clearFilter())
	assert.Equal(t, 4, visibleRowCount(table.view(120)))
	assert.False(t, table.clearFilter())
}

// Space reaches the filter like any other printable key. It arrives as its
// own key type, never inside KeyRunes, and the table underneath binds it to
// page-down — so before this was handled, a name like "Py 3.12" could not be
// typed and the keystroke silently paged the list instead.
func TestRowTable_FilterAcceptsSpaces(t *testing.T) {
	rows := []tableRow{
		{cells: []string{"Py 3.12", "a"}, value: 1},
		{cells: []string{"Py 3.9", "b"}, value: 2},
		{cells: []string{"Python-nospace", "c"}, value: 3},
	}

	table := newRowTable([]string{"NAME", "DETAIL"}, rows, true, 120, 44)

	typeFilter(&table, "py")
	assert.True(t, table.update(tea.KeyMsg{Type: tea.KeySpace}), "space is filter input, not paging")
	typeFilter(&table, "3.1")

	assert.Equal(t, "py 3.1", table.filter)
	assert.Equal(t, 1, visibleRowCount(table.view(120)), "only 'Py 3.12' carries the spaced query")

	// One backspace removes the space like any other character.
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	table.update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "py", table.filter)
	assert.Equal(t, 3, visibleRowCount(table.view(120)))
}

// A filter that matches nothing says so, rather than showing an empty frame.
func TestRowTable_FilterMatchingNothing(t *testing.T) {
	table := newRowTable([]string{"NAME", "DETAIL"}, fakeRows(3), true, 120, 44)

	typeFilter(&table, "zzz")

	assert.Contains(t, table.view(120), "nothing matches")
	assert.Nil(t, table.selected())
}

// The cursor addresses the filtered rows, so selecting after a filter returns
// the row the user is actually looking at.
func TestRowTable_SelectionFollowsTheFilter(t *testing.T) {
	table := newRowTable([]string{"NAME", "DETAIL"}, fakeRows(20), true, 120, 44)

	typeFilter(&table, "row-19")

	row := table.selected()
	require.NotNil(t, row)
	assert.Equal(t, 19, row.value)
}

// A list longer than the window says where you are in it.
func TestRowTable_ReportsPositionWhenScrolling(t *testing.T) {
	table := newRowTable([]string{"NAME", "DETAIL"}, fakeRows(200), true, 120, 44)

	assert.Contains(t, table.view(120), fmt.Sprintf("1 of %d", 200))

	table.update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Contains(t, table.view(120), fmt.Sprintf("2 of %d", 200))
}

// One very long value must not push the other columns off the screen.
func TestColumnsFor_CapsAnOutlier(t *testing.T) {
	long := strings.Repeat("x", 200)
	columns := columnsFor([]string{"NAME", "ID"},
		[]tableRow{{cells: []string{long, "67a554bbfef3a4ce2ab6700"}}}, 120)

	// Column 0 is the cursor marker, so the data columns start at 1.
	assert.LessOrEqual(t, columns[1].Width, maxColumnWidth)
	assert.Equal(t, len("67a554bbfef3a4ce2ab6700"), columns[2].Width,
		"the id column keeps its natural width")
}
