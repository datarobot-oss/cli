package tui

import tea "github.com/charmbracelet/bubbletea"

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
