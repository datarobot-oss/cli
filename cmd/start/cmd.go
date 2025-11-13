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
	"github.com/datarobot/cli/cmd/templates/setup"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Options struct {
	AnswerYes bool
}

// updateStateAfterSuccess updates the state file after a successful dr start run.
func updateStateAfterSuccess() error {
	return state.UpdateAfterSuccessfulRun()
}

func Cmd() *cobra.Command {
	var opts Options

	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"quickstart"},
		GroupID: "core",
		Short:   "Run the application quickstart process",
		Long: `Run the application quickstart process for the current template.
The following actions will be performed:
- Checking for prerequisite tooling
- Validating the environment (TODO)
- Executing the quickstart script associated with the template, if available.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if viper.GetBool("debug") {
				f, err := tea.LogToFile("tea-debug.log", "debug")
				if err != nil {
					fmt.Println("fatal:", err)
					os.Exit(1)
				}

				defer f.Close()
			}

			m := NewStartModel(opts)
			p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())

			finalModel, err := p.Run()
			if err != nil {
				return err
			}

			// Check if we need to launch template setup after quitting
			if startModel, ok := finalModel.(tui.InterruptibleModel); ok {
				if innerModel, ok := startModel.Model.(Model); ok {
					if innerModel.quickstartScriptPath == "" && innerModel.done && !innerModel.quitting {
						// No quickstart found, will launch template setup
						// Update state before launching setup
						_ = updateStateAfterSuccess()

						return setup.RunTeaFromStart(cmd.Context(), true)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")

	return cmd
}
