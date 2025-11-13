// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("should be in repository root directory")
	}

	// Do we have the required tools?
	//if err := tools.CheckPrerequisites(); err != nil {
	//	return err
	//}

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		am := NewAddModel()
		p := tea.NewProgram(tui.NewInterruptibleModel(am), tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		// Check if we need to launch template setup after quitting
		if startModel, ok := finalModel.(tui.InterruptibleModel); ok {
			if innerModel, ok := startModel.Model.(AddModel); ok {
				args = innerModel.RepoURLs
			}
		}
	}

	if len(args) == 0 || args[0] == "" {
		return errors.New("component_url required")
	}

	for _, repoURL := range args {
		fmt.Printf("Adding component: %s\n", repoURL)

		err := copier.ExecAdd(repoURL)
		if err != nil {
			log.Error(err)
			os.Exit(1)

			return nil
		}

		fmt.Printf("Component %s added\n", repoURL)
	}

	compose.Run(nil, nil)

	return nil
}

var AddCmd = &cobra.Command{
	Use:     "add [component_url]",
	Short:   "Add component",
	PreRunE: PreRunE,
	RunE:    RunE,
}
