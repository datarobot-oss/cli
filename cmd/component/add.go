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
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
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

func RunE(cmd *cobra.Command, args []string) error {
	args, err := getArgsFromCLIOrPrompt(args)
	if err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "" {
		return errors.New("A component URL is required.")
	}

	// Parse --data arguments
	dataArgs, _ := cmd.Flags().GetStringArray("data")

	cliData, err := parseDataArgs(dataArgs)
	if err != nil {
		log.Error(err)
		os.Exit(1)

		return nil
	}

	// Get --data-file path if specified
	dataFile, _ := cmd.Flags().GetString("data-file")

	componentConfig := loadComponentDefaults(dataFile)

	if err := addComponents(args, componentConfig, cliData); err != nil {
		return err
	}

	compose.Run(nil, nil)

	return nil
}

func getArgsFromCLIOrPrompt(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}

	am := NewAddModel()
	p := tea.NewProgram(tui.NewInterruptibleModel(am), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	// Check if we need to launch template setup after quitting
	if startModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := startModel.Model.(AddModel); ok {
			return innerModel.RepoURLs, nil
		}
	}

	return args, nil
}

func loadComponentDefaults(dataFilePath string) *config.ComponentDefaults {
	componentConfig, err := config.LoadComponentDefaults(dataFilePath)
	if err != nil {
		log.Warn("Failed to load component defaults", "error", err)

		componentConfig = &config.ComponentDefaults{
			Defaults: make(map[string]map[string]interface{}),
		}
	}

	return componentConfig
}

func addComponents(repoURLs []string, componentConfig *config.ComponentDefaults, cliData map[string]interface{}) error {
	for _, repoURL := range repoURLs {
		fmt.Printf("Adding component: %s.\n", repoURL)

		// Merge defaults with CLI data (CLI data takes precedence)
		mergedData := componentConfig.MergeWithCLIData(repoURL, cliData)

		var execErr error
		if len(mergedData) > 0 {
			execErr = copier.ExecAddWithData(repoURL, mergedData)
		} else {
			execErr = copier.ExecAdd(repoURL)
		}

		if execErr != nil {
			log.Error(execErr)
			os.Exit(1)

			return nil
		}

		fmt.Printf("Component %s added.\n", repoURL)
	}

	return nil
}

var AddCmd = &cobra.Command{
	Use:     "add [component_url]",
	Short:   "Add a component.",
	PreRunE: PreRunE,
	RunE:    RunE,
}

func init() {
	AddCmd.Flags().StringArrayP("data", "d", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	AddCmd.Flags().String("data-file", "", "Path to YAML file with default answers (follows copier data_file semantics)")
}
