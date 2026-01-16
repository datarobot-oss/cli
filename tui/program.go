package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// DebugLogFile is the filename for TUI debug logs
const DebugLogFile = "dr-tui-debug.log"

// Run is a wrapper for tea.NewProgram and (p *Program) Run()
// Configures debug logging for the TUI if debug mode is enabled
// Wraps a model in NewInterruptibleModel
func Run(model tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
	if viper.GetBool("debug") {
		f, err := tea.LogToFile(DebugLogFile, "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}

		log.SetStyles(DebugLogStyles)
		log.SetOutput(f)

		defer f.Close()
	}

	p := tea.NewProgram(NewInterruptibleModel(model), opts...)

	finalModel, err := p.Run()

	return finalModel, err
}
