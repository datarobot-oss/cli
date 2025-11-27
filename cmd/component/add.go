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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/gitsight/go-vcsurl"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	// Do we have the required tools?
	//if err := tools.CheckPrerequisites(); err != nil {
	//	return err
	//}

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	if len(args) == 0 || args[0] == "" {
		var err error

		args, err = AddRunTea()
		if err != nil {
			return err
		}
	}

	if len(args) == 0 || args[0] == "" {
		return errors.New("A component URL is required.")
	}

	for _, repoURL := range args {
		if component, ok := copier.ComponentDetailsByShortName[repoURL]; ok {
			repoURL = component.RepoURL
		}

		_, repoErr := vcsurl.Parse(repoURL)
		if repoErr != nil {
			log.Errorf("Skipping component \"%s\": invalid url (%s)", repoURL, repoErr)
			continue
		}

		fmt.Printf("Adding component: %s.\n", repoURL)

		err := copier.ExecAdd(repoURL)
		if err != nil {
			log.Error(err)
			os.Exit(1)

			return nil
		}

		fmt.Printf("Component %s added.\n", repoURL)
	}

	compose.Cmd().Run(nil, nil)

	return nil
}

func AddRunTea() ([]string, error) {
	am := NewAddModel()
	p := tea.NewProgram(tui.NewInterruptibleModel(am), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	// Get list of components that user selected
	if addModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := addModel.Model.(AddModel); ok {
			if len(innerModel.RepoURLs) > 0 {
				return innerModel.RepoURLs, nil
			}
		}
	}

	return nil, nil
}

func AddCmd() *cobra.Command {
	names := strings.Join(copier.EnabledShortNames, ", ")

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("add [%s or component_url]", names),
		Short:   "Add a component.",
		PreRunE: PreRunE,
		RunE:    RunE,
	}

	return cmd
}
