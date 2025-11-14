// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func ListRunE(_ *cobra.Command, _ []string) error {
	if viper.GetBool("debug") {
		f, err := tea.LogToFile("tea-debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}

		defer f.Close()
	}

	m := NewComponentModel(listCmd, listScreen)
	p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List components.",
	RunE:  ListRunE,
}
