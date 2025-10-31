// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Cmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup template configuration (interactive mode)",
	Long: `Setup and configure the current template with an interactive setup wizard.

This interactive command:
- Helps with setting up the template configuration

This command launches an interactive terminal interface to guide you through
the template configuration process step by step.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return RunTea(cmd.Context())
	},
}

// RunTea starts the template setup TUI
func RunTea(ctx context.Context) error {
	if viper.GetBool("debug") {
		f, err := tea.LogToFile(tui.DebugLogFile, "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
	}

	m := NewModel()
	p := tea.NewProgram(
		tui.NewInterruptibleModel(m),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := p.Run()

	return err
}
