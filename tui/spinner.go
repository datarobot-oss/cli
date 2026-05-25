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

type spinnerModel struct {
	spinner spinner.Model
	label   string
	fn      func() error
	done    bool
}

type spinnerDoneMsg struct{ err error }

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			return spinnerDoneMsg{err: m.fn()}
		},
	)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerDoneMsg:
		m.done = true

		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}

	return InfoStyle.Render(m.spinner.View()+" ") + m.label + "\n"
}

// RunWithSpinner runs fn in the background while showing an animated spinner
// with the given label. Returns the error from fn, if any.
func RunWithSpinner(label string, fn func() error) error {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = InfoStyle

	var fnErr error

	m := spinnerModel{
		spinner: s,
		label:   label,
		fn: func() error {
			fnErr = fn()

			return fnErr
		},
	}

	_, err := Run(m)
	if err != nil {
		return err
	}

	return fnErr
}
