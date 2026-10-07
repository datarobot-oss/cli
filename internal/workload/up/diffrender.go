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

package up

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

// lineKind classifies one line of a diff body.
type lineKind int

const (
	// lineContext is a row the file names that the live object already
	// agrees with. It shows within the window of a change and collapses
	// into a summary beyond it.
	lineContext lineKind = iota

	// lineAdd is a value arriving, rendered with a "+".
	lineAdd

	// lineDel is a value leaving, rendered with a "-".
	lineDel
)

// diffLine is one line of a diff body: the text is already formatted, and
// the path exists for the redaction hook alone.
type diffLine struct {
	kind lineKind
	path string
	text string
}

// contextWindow is the context window git taught everyone to read: enough
// surrounding rows to place a change, not so many that the diff stops being
// shorter than the thing it diffs. There is no flag for it, by design.
const contextWindow = 3

// The words that stand in for a value the redaction hook refused to print.
// "set" and "changed" are the plan's own vocabulary for a value arriving
// and a value moving; the bracketed token marks a context row whose value
// is simply withheld.
const (
	setPlaceholder     = "set"
	changedPlaceholder = "changed"
	hiddenPlaceholder  = "(redacted)"
)

// renderUnified writes lines as a unified diff. Every change shows, and so
// does every context line within the window of one; the rest of each
// context run collapses into one "... N identical lines" line that counts
// what it hides. A line whose path redact refuses prints a placeholder in
// place of its text, whatever its kind.
func renderUnified(w io.Writer, lines []diffLine, redact func(string) bool) error {
	shown := shownLines(lines)

	var b strings.Builder

	for i := 0; i < len(lines); {
		if shown[i] {
			b.WriteString(renderLine(lines[i], redact))
			b.WriteString("\n")

			i++

			continue
		}

		hidden := 0

		for i+hidden < len(lines) && !shown[i+hidden] {
			hidden++
		}

		b.WriteString(tui.HintStyle.Render("... " + strconv.Itoa(hidden) + " identical lines"))
		b.WriteString("\n")

		i += hidden
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("cannot write diff: %w", err)
	}

	return nil
}

// shownLines marks every change and every line within the window of one.
func shownLines(lines []diffLine) []bool {
	shown := make([]bool, len(lines))

	for i, line := range lines {
		if line.kind == lineContext {
			continue
		}

		lo := max(i-contextWindow, 0)
		hi := min(i+contextWindow, len(lines)-1)

		for j := lo; j <= hi; j++ {
			shown[j] = true
		}
	}

	return shown
}

// renderLine is one shown line: its marker and text, with the redaction
// hook standing in for the text of a path whose value must not print.
func renderLine(line diffLine, redact func(string) bool) string {
	text := line.text

	if redact != nil && redact(line.path) {
		text = redactedText(line)
	}

	switch line.kind {
	case lineAdd:
		return tui.SuccessStyle.Render("+ " + text)
	case lineDel:
		return tui.WarnStyle.Render("- " + text)
	case lineContext:
		return tui.HintStyle.Render(text)
	}

	return tui.HintStyle.Render(text)
}

// redactedText is what a line prints when its value must not: the path,
// plus the word its kind calls for.
func redactedText(line diffLine) string {
	switch line.kind {
	case lineAdd:
		return line.path + ": " + setPlaceholder
	case lineDel:
		return line.path + ": " + changedPlaceholder
	case lineContext:
		return line.path + ": " + hiddenPlaceholder
	}

	return line.path + ": " + hiddenPlaceholder
}

// lineStyle is the palette entry a kind renders with, for tests that pin
// the colours.
func lineStyle(kind lineKind) lipgloss.Style {
	switch kind {
	case lineAdd:
		return tui.SuccessStyle
	case lineDel:
		return tui.WarnStyle
	case lineContext:
		return tui.HintStyle
	}

	return tui.HintStyle
}
