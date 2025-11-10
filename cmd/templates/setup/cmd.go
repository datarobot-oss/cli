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
	Short: "🎉 Interactive template setup wizard",
	Long: `Launch the interactive template setup wizard to get started with DataRobot AI applications.

🎯 This wizard will help you:
  1️⃣  Choose an AI application template
  2️⃣  Clone it to your computer  
  3️⃣  Configure your environment
  4️⃣  Get you ready to build!

⏱️  Takes about 3-5 minutes
🎉  You'll have a working AI app at the end

💡 Perfect for first-time users or someone starting a new project.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return RunTea(cmd.Context())
	},
}

// RunTea starts the template setup TUI
func RunTea(ctx context.Context) error {
	if viper.GetBool("debug") {
		f, err := tea.LogToFile("tea-debug.log", "debug")
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
