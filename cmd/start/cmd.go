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
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Options struct {
	AnswerYes bool
}

func Cmd() *cobra.Command { //nolint: cyclop
	var opts Options

	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"quickstart"},
		GroupID: "core",
		Short:   "🚀 Run the application quickstart process",
		Long: `Run the application quickstart process for the current template.
The following actions will be performed:
- Checking for prerequisite tooling
- Executing the start script associated with the template, if available.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if viper.GetBool("debug") {
				f, err := tea.LogToFile("tea-debug.log", "debug")
				if err != nil {
					fmt.Println("fatal: ", err)
					os.Exit(1)
				}

				defer f.Close()
			}

			m := NewStartModel(opts)
			p := tea.NewProgram(tui.NewInterruptibleModel(m))

			finalModel, err := p.Run()
			if err != nil {
				return err
			}

			innerModel, ok := getInnerModel(finalModel)
			if !ok {
				return nil
			}

			if innerModel.err != nil {
				os.Exit(1)
			}

			// Check if we need to launch template setup after quitting
			if innerModel.needTemplateSetup && innerModel.done && !innerModel.quitting {
				// Need to run template setup
				// After it completes, we'll be in the cloned directory,
				// so we can just run start again
				err := setup.RunTea(cmd.Context(), true)
				if err != nil {
					return err
				}

				// Now run start again - we're in the cloned repo directory
				// Create a new start model and run it
				m2 := NewStartModel(opts)
				p2 := tea.NewProgram(tui.NewInterruptibleModel(m2))

				finalModel2, err := p2.Run()
				if err != nil {
					return err
				}

				innerModel2, ok := getInnerModel(finalModel2)
				if !ok {
					return nil
				}

				if innerModel2.err != nil {
					os.Exit(1)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")

	return cmd
}

func getInnerModel(finalModel tea.Model) (Model, bool) {
	startModel, ok := finalModel.(tui.InterruptibleModel)
	if !ok {
		return Model{}, false
	}

	innerModel, ok := startModel.Model.(Model)
	if !ok {
		return Model{}, false
	}

	return innerModel, true
}
