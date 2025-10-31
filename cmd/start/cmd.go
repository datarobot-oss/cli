// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package start

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type startOptions struct {
	AnswerYes bool
}

func Cmd() *cobra.Command {
	var opts startOptions

	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"quickstart"},
		GroupID: "core",
		Short:   "Run the application quickstart process",
		Long: `Run the application quickstart process for the current template.
Running this command performs the following actions:
- Validating the environment
- Checking template prerequisites
- Executing the quickstart script associated with the template, if available.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if viper.GetBool("debug") {
				f, err := tea.LogToFile("tea-debug.log", "debug")
				if err != nil {
					fmt.Println("fatal:", err)
					os.Exit(1)
				}
				defer f.Close()
			}

			m := NewStartModel()
			p := tea.NewProgram(tui.NewInterruptibleModel(m))

			if _, err := p.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")

	return cmd
}
