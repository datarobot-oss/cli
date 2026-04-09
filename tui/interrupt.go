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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/log"
)

// InterruptibleModel wraps any Bubble Tea model to ensure Ctrl-C always works.
// This wrapper intercepts ALL messages before they reach the underlying model,
// checking for Ctrl-C and immediately quitting if detected. This guarantees
// users can never get stuck in the program, regardless of what the model does.
type InterruptibleModel struct {
	Model      tea.Model
	konami     konamiDetector
	rocket     *RocketModel
	termWidth  int
	termHeight int
}

// NewInterruptibleModel wraps a model to ensure Ctrl-C always works everywhere.
// Use this when creating any Bubble Tea program to guarantee users can exit.
//
// Example:
//
//	m := myModel{}
//	p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())
func NewInterruptibleModel(model tea.Model) InterruptibleModel {
	return InterruptibleModel{Model: model}
}

func (m InterruptibleModel) Init() tea.Cmd {
	return m.Model.Init()
}

func (m InterruptibleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Track terminal size for rocket animation
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = sizeMsg.Width
		m.termHeight = sizeMsg.Height
	}

	// Universal Ctrl-C handling - ALWAYS checked FIRST before any model logic
	// This ensures users can always interrupt, regardless of nested components,
	// screen state, or what the underlying model does
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "ctrl+c" {
			// Log the interrupt for debugging purposes
			log.Info("Ctrl-C detected, quitting...")

			return m, tea.Quit
		}
	}

	// When the rocket animation is running, route messages to it
	if m.rocket != nil {
		if _, ok := msg.(RocketDoneMsg); ok {
			m.rocket = nil

			return m, nil
		}

		updated, cmd := m.rocket.Update(msg)
		rocket := updated.(RocketModel)
		m.rocket = &rocket

		return m, cmd
	}

	// Check for Konami code on key presses
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if cmd := m.handleKonami(keyMsg); cmd != nil {
			return m, cmd
		}
	}

	// Pass the message to the wrapped model
	updatedModel, cmd := m.Model.Update(msg)

	// Keep the wrapper around the updated model
	m.Model = updatedModel

	return m, cmd
}

func (m *InterruptibleModel) handleKonami(keyMsg tea.KeyMsg) tea.Cmd {
	if !m.konami.Feed(keyMsg) {
		return nil
	}

	log.Info("Konami code activated!")

	w := max(m.termWidth, 80)
	h := max(m.termHeight, 24)

	rocket := newRocketModel(w, h)
	m.rocket = &rocket

	return rocket.Init()
}

func (m InterruptibleModel) View() string {
	if m.rocket != nil {
		return m.rocket.View()
	}

	return m.Model.View()
}
