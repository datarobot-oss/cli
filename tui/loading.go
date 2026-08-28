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
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Loading is an embeddable spinner sub-model for use inside larger Bubble Tea
// models (unlike RunWithSpinner, which owns its own tea.Program and isn't
// meant to be nested inside another model's Update loop).
//
// bubbles/spinner stamps every spinner.TickMsg with the emitting spinner's
// own ID, and spinner.Model.Update no-ops (no rescheduled tick) when the ID
// doesn't match. So Update is always safe to call with any message a parent
// forwards down, including ticks belonging to a sibling or child's own
// Loading. What Loading does NOT do is guarantee delivery: a parent model
// must still forward every message to its active child before/alongside
// handling its own spinner, rather than consuming a message by type alone
// and returning early - otherwise a child's ticks never arrive and its
// spinner freezes after one frame.
type Loading struct {
	Spinner spinner.Model
}

// NewLoading creates a Loading using the CLI's standard spinner glyph/style.
func NewLoading() Loading {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = InfoStyle

	return Loading{Spinner: s}
}

// Init starts the spinner ticking. Callers must include the returned command,
// typically batched with any other work started concurrently.
func (l Loading) Init() tea.Cmd {
	return l.Spinner.Tick
}

// Update advances the spinner on its own tick messages and no-ops for
// anything else.
func (l Loading) Update(msg tea.Msg) (Loading, tea.Cmd) {
	var cmd tea.Cmd

	l.Spinner, cmd = l.Spinner.Update(msg)

	return l, cmd
}

// View renders the spinner glyph.
func (l Loading) View() string {
	return l.Spinner.View()
}
