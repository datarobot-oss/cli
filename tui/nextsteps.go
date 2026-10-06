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

package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// NextStep is one command a Next: block suggests, and what running it does.
type NextStep struct {
	Command     string
	Description string
}

// PrintNextSteps writes the Next: block a command ends with when it has
// something worth running afterwards: one command per line, set off from its
// description so it reads as something to copy rather than as prose. Nothing
// is written for no steps, so a caller whose list came out empty need not
// check first.
//
// It is for the end of a command, not for help with a question it is about
// to ask: a Next: header above a prompt tells the reader the command is done
// while it is still waiting for them.
//
// Callers pass stderr, so the result on stdout stays pipeable, and do not call
// it under --output-format json, where nothing but the envelope is wanted.
func PrintNextSteps(w io.Writer, steps ...NextStep) {
	if len(steps) == 0 {
		return
	}

	// Bound to w, which is stderr and may be a file while stdout is a
	// terminal. Tabs are left alone: a command is copied as printed, and
	// lipgloss would otherwise turn a tab inside a quoted path into spaces,
	// naming a directory that does not exist.
	styles := StylesFor(w)
	hint := styles.Hint
	command := styles.Info.TabWidth(lipgloss.NoTabConversion)

	// The padding is measured on the bare command and written outside its
	// style, so the descriptions line up in a column whether or not the
	// terminal renders the escape codes.
	width := 0

	for _, step := range steps {
		width = max(width, lipgloss.Width(step.Command))
	}

	fmt.Fprintf(w, "\n%s\n", hint.Render("Next:"))

	for _, step := range steps {
		pad := strings.Repeat(" ", width-lipgloss.Width(step.Command))

		fmt.Fprintf(w, "  %s%s  %s\n", command.Render(step.Command), pad, hint.Render(step.Description))
	}
}
