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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
)

// option is one answer on a choice screen.
type option struct {
	// value is what the answer means to the draft.
	value string
	label string
	// note sits to the right of the label, for the aside that makes the
	// choice obvious: which one is usual, what was detected.
	note string
	// warn marks a note that is a reason this option will not work here, so
	// it reads as a warning rather than as a detail.
	warn bool
}

// choice is the two-or-three answer screen the wizard is mostly made of:
// arrow keys or a digit, Enter to accept. bubbles/list is the wrong shape
// here, being built for long, filterable collections; these screens are a
// numbered menu the user reads in one glance.
type choice struct {
	options  []option
	selected int
	// liveValue is what the bound workload uses today, "" on a fresh setup.
	// The row carrying it is tagged, because preselecting it says which
	// option the wizard would pick and not which one is already true.
	liveValue string
}

// newChoice builds the screen with current preselected. A current value the
// screen has no option for is not silently replaced by the first row: it gets
// a row of its own, so a workload running something this release cannot
// author is kept rather than converted by a single Enter.
func newChoice(options []option, current string, keepLabel string) choice {
	if current != "" && !hasOption(options, current) {
		options = append([]option{{
			value: current,
			label: keepLabel,
			note:  "leave it as " + current + "; this wizard cannot author that kind",
		}}, options...)
	}

	c := choice{options: options}

	for i, opt := range options {
		if opt.value == current {
			c.selected = i
		}
	}

	return c
}

func hasOption(options []option, value string) bool {
	for _, opt := range options {
		if opt.value == value {
			return true
		}
	}

	return false
}

// update moves the cursor and reports whether the key was consumed, so the
// caller can treat everything else (Enter, esc) as navigation.
func (c *choice) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		if c.selected > 0 {
			c.selected--
		}

		return true
	case "down", "j":
		if c.selected < len(c.options)-1 {
			c.selected++
		}

		return true
	}

	// A digit picks its row outright, which is how a numbered menu is meant
	// to be used.
	if index := digit(msg.String()); index > 0 && index <= len(c.options) {
		c.selected = index - 1

		return true
	}

	return false
}

func (c choice) value() string {
	if len(c.options) == 0 {
		return ""
	}

	return c.options[c.selected].value
}

func (c choice) view() string {
	var b strings.Builder

	for i, opt := range c.options {
		line := fmt.Sprintf("%d. %s", i+1, opt.label)

		if i == c.selected {
			// The same cursor glyph the pickers use (table.go syncRows), so a
			// selection reads the same across menus and lists.
			b.WriteString(selectedStyle.Render("❯ " + line))
		} else {
			b.WriteString("  " + line)
		}

		if opt.note != "" {
			style := tui.HintStyle
			if opt.warn {
				style = tui.ErrorStyle
			}

			b.WriteString(style.Render("   " + opt.note))
		}

		// A trailing green tag on the right, its own colour whatever the option
		// is doing: green means "from the running workload" throughout the
		// wizard's notes, and the tag is one more live-derived value.
		if c.liveValue != "" && opt.value == c.liveValue {
			b.WriteString(liveStyle.Render("   · in use"))
		}

		b.WriteString("\n")
	}

	return b.String()
}

func digit(key string) int {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0
	}

	return int(key[0] - '0')
}
