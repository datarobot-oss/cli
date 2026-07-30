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

package hostpicker

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
)

// chosenMsg carries the URL the user selected.
type chosenMsg struct{ url string }

// standaloneModel adapts Model, which is designed to be embedded in a larger
// wizard, into a program that can run on its own and then exit.
//
// It owns the two behaviours the embedded model deliberately leaves to its
// parent: turning a selection into a quit, and treating Esc in list mode as
// "cancel" rather than as the custom-input "go back".
type standaloneModel struct {
	picker   Model
	url      string
	chosen   bool
	quitting bool
}

func newStandaloneModel() standaloneModel {
	picker := New()
	picker.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg { return chosenMsg{url: url} }
	}

	return standaloneModel{picker: picker}
}

func (m standaloneModel) Init() tea.Cmd {
	return m.picker.Init()
}

func (m standaloneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case chosenMsg:
		m.url = msg.url
		m.chosen = true
		m.quitting = true

		return m, tea.Quit

	case tea.KeyMsg:
		// Esc inside the custom-URL field means "back to the list"; the embedded
		// model handles that itself. Only Esc in list mode cancels the picker.
		if msg.Type == tea.KeyEsc && !m.picker.showCustom {
			m.quitting = true

			return m, tea.Quit
		}
	}

	var cmd tea.Cmd

	m.picker, cmd = m.picker.Update(msg)

	return m, cmd
}

func (m standaloneModel) View() string {
	if m.quitting {
		return ""
	}

	return m.picker.View()
}

// Select runs the environment picker as its own program and returns the URL the
// user chose.
//
// ok is false when the user cancelled with Esc or Ctrl-C. Callers must check it
// before using the URL; a cancel is a normal outcome, not an error.
func Select() (url string, ok bool, err error) {
	finalModel, err := tui.Run(newStandaloneModel())
	if err != nil {
		return "", false, err
	}

	final, isStandalone := tui.Unwrap(finalModel).(standaloneModel)
	if !isStandalone {
		return "", false, nil
	}

	return final.url, final.chosen, nil
}
