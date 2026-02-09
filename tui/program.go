package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/log"
)

// Run is a wrapper for tea.NewProgram and (p *Program) Run()
// Disables stderr logging while bubbletea program is running
// Wraps a model in NewInterruptibleModel
func Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
	// Pause stderr logger to prevent breaking of bubbletea program output
	log.StopStderr()

	defer log.StartStderr()

	p := tea.NewProgram(NewInterruptibleModel(model), opts...)
	finalModel, err := p.Run()

	return finalModel, err
}
