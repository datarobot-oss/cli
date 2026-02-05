package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/log"
)

// Run is a wrapper for tea.NewProgram and (p *Program) Run()
// Configures debug logging for the TUI if debug mode is enabled
// Wraps a model in NewInterruptibleModel
func Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
	log.StopStderr()

	p := tea.NewProgram(NewInterruptibleModel(model), opts...)
	finalModel, err := p.Run()

	log.StartStderr()

	return finalModel, err
}
