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
	Model tea.Model
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

	// Pass the message to the wrapped model
	updatedModel, cmd := m.Model.Update(msg)

	// Keep the wrapper around the updated model
	m.Model = updatedModel

	return m, cmd
}

func (m InterruptibleModel) View() string {
	return m.Model.View()
}
