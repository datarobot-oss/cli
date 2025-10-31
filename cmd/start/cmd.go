// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package start

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
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
			m := NewModel()
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
