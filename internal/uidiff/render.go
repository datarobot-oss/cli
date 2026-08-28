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

package uidiff

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

// defaultContextWindow is the context window git taught everyone to read:
// enough surrounding rows to place a change, not so many that the diff stops
// being shorter than the thing it diffs.
const defaultContextWindow = 3

// The marker characters are the vocabulary the plan block already prints, so
// a diff and the default plan read as one visual language. Context and
// unmanaged rows add no marker: their text arrives from the caller complete.
const (
	addMarker = "+"
	delMarker = "-"
)

// The words that stand in for a value the redaction hook refused to print.
// "set" and "changed" are the plan's own vocabulary for a value arriving and
// a value moving; the bracketed token marks rows whose value is simply
// withheld, where neither verb is true.
const (
	setPlaceholder     = "set"
	changedPlaceholder = "changed"
	hiddenPlaceholder  = "(redacted)"
)

// Render writes rows as a unified diff to w.
//
// Every change shows, and so does every unchanged row within the context
// window of one. The rest of each unchanged run collapses into a single
// "... N identical lines" line that counts only the rows it hides, so the
// rendering is always shorter than the row list it came from and never
// understates how much it left out. Redacted rows render as a placeholder
// built from their path, whatever their Kind: no value text reaches the
// writer for a path the hook refuses.
func Render(w io.Writer, rows []Row, opts Options) error {
	window := opts.Context
	if window <= 0 {
		window = defaultContextWindow
	}

	shown := shownRows(rows, window)

	var b strings.Builder

	for i := 0; i < len(rows); {
		if shown[i] {
			b.WriteString(renderRow(rows[i], opts.Redact))
			b.WriteString("\n")

			i++

			continue
		}

		hidden := 0

		for i+hidden < len(rows) && !shown[i+hidden] {
			hidden++
		}

		b.WriteString(collapseLine(hidden))
		b.WriteString("\n")

		i += hidden
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("cannot write diff: %w", err)
	}

	return nil
}

// shownRows marks which rows the window keeps: every change, plus every row
// within the window of one. Distance is measured in rows, not in equality,
// because the collapse answers "how much of this did I stop reading?" and
// that question does not care what the hidden rows say.
func shownRows(rows []Row, window int) []bool {
	shown := make([]bool, len(rows))

	for i, row := range rows {
		if row.Kind != Add && row.Kind != Del {
			continue
		}

		lo := max(i-window, 0)
		hi := min(i+window, len(rows)-1)

		for j := lo; j <= hi; j++ {
			shown[j] = true
		}
	}

	return shown
}

// collapseLine is the one line that stands for a hidden run. It counts the
// rows it hides rather than the ones it kept, so a reader can always tell
// how much output did not happen.
func collapseLine(hidden int) string {
	return tui.HintStyle.Render("... " + strconv.Itoa(hidden) + " identical lines")
}

// renderRow renders one shown row: the marker and text its kind calls for,
// with the redaction hook standing in for the text of a row whose value must
// not print.
func renderRow(row Row, redact func(string) bool) string {
	marker, text := rowContent(row, redact)

	if marker == "" {
		return rowStyle(row.Kind).Render(text)
	}

	return rowStyle(row.Kind).Render(marker + " " + text)
}

// rowContent decides what a row prints. The caller's Text is the answer for
// everything but a redacted path, where the value inside the caller's text
// is replaced by the placeholder its kind calls for: the path stays, the
// value never does. The hook is consulted once per row, here, so a caller
// with an expensive predicate pays for it once.
func rowContent(row Row, redact func(string) bool) (marker, text string) {
	text = row.Text

	if redact != nil && redact(row.Path) {
		text = redactedText(row)
	}

	switch row.Kind {
	case Add:
		return addMarker, text
	case Del:
		return delMarker, text
	case Context, Unmanaged:
		return "", text
	}

	// An out-of-range Kind has no marker of its own; it renders as context
	// rather than inventing one.
	return "", text
}

// redactedText is what a row prints when its value must not: the path, plus
// the word its kind calls for. "set" and "changed" are the plan's own
// vocabulary for a value arriving and a value moving; the bracketed token
// marks rows whose value is simply withheld, where neither verb is true.
func redactedText(row Row) string {
	switch row.Kind {
	case Add:
		return row.Path + ": " + setPlaceholder
	case Del:
		return row.Path + ": " + changedPlaceholder
	case Context, Unmanaged:
		return row.Path + ": " + hiddenPlaceholder
	}

	return row.Path + ": " + hiddenPlaceholder
}

// rowStyle is the palette entry for a row's kind. An addition reads as new,
// a replacement and an unmanaged field as something being moved, and
// everything merely contextual as commentary on it.
func rowStyle(kind Kind) lipgloss.Style {
	switch kind {
	case Add:
		return tui.SuccessStyle
	case Del, Unmanaged:
		return tui.WarnStyle
	case Context:
		return tui.HintStyle
	}

	// An out-of-range Kind reads as context, quieter than guessing wrong.
	return tui.HintStyle
}
