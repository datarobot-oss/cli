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
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/log"
)

// ErrInterrupted reports that the user pressed Ctrl-C. It exists because
// Bubble Tea cannot say so on its own: the wrapper below answers an interrupt
// with tea.Quit, tea.Quit becomes a QuitMsg, and a QuitMsg ends Program.Run
// with a nil error — the same nil a model that finished its work returns. A
// caller that only reads that error cannot tell "the user gave up" from "the
// work is done", which is how an abandoned deploy came to be reported as a
// healthy one (RAPTOR-19963).
var ErrInterrupted = errors.New("interrupted")

// InterruptibleModel wraps any Bubble Tea model to ensure Ctrl-C always works.
// This wrapper intercepts ALL messages before they reach the underlying model,
// checking for Ctrl-C and immediately quitting if detected. This guarantees
// users can never get stuck in the program, regardless of what the model does.
type InterruptibleModel struct {
	Model tea.Model

	// interrupted records that the quit came from Ctrl-C rather than from the
	// wrapped model finishing. Bubble Tea hands the final model back from
	// Program.Run, so setting it here is what lets WasInterrupted answer the
	// question afterwards.
	interrupted bool
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

			m.interrupted = true

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

// Unwrap returns the wrapped model, so callers that need to recover the
// original concrete model type after tui.Run() can peel this layer off.
// See Unwrap (package-level) for why this exists.
func (m InterruptibleModel) Unwrap() tea.Model {
	return m.Model
}

// WasInterrupted reports whether the program tui.Run() just finished ended on
// Ctrl-C. It reads the outermost wrapper rather than peeling layers, because
// the interrupt is recorded by the wrapper Run() puts on the outside.
//
// Callers opt in: Run() keeps returning a nil error for an interrupt, so a
// model that already treats Ctrl-C as its own kind of cancellation is
// unaffected. A caller that runs work behind the model, where quitting early
// means the work never finished, asks this instead of assuming success.
func WasInterrupted(final tea.Model) bool {
	m, ok := final.(InterruptibleModel)

	return ok && m.interrupted
}
