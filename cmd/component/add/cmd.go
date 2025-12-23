// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package add

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/gitsight/go-vcsurl"
	"github.com/spf13/cobra"
)

var addFlags copier.AddFlags

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	args, err := getArgsFromCLIOrPrompt(args)
	if err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "" {
		return errors.New("A component URL is required.")
	}

	cliData, err := shared.ParseDataArgs(addFlags.DataArgs)
	if err != nil {
		log.Error(err)
		os.Exit(1)

		return nil
	}

	componentConfig := loadComponentDefaults(addFlags.DataFile)

	if err := addComponents(args, componentConfig, cliData); err != nil {
		return err
	}

	compose.Cmd().Run(nil, nil)

	return nil
}

func getArgsFromCLIOrPrompt(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}

	am := shared.NewAddModel()
	p := tea.NewProgram(tui.NewInterruptibleModel(am), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	// Check if we need to launch template setup after quitting
	if startModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := startModel.Model.(shared.AddModel); ok {
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
		if component, ok := copier.ComponentDetailsByShortName[repoURL]; ok {
			repoURL = component.RepoURL
		}

		_, repoErr := vcsurl.Parse(repoURL)
		if repoErr != nil {
			log.Errorf("Skipping component \"%s\": invalid url (%s)", repoURL, repoErr)
			continue
		}

		fmt.Printf("Adding component: %s.\n", repoURL)

		// Merge defaults with CLI data (CLI data takes precedence)
		mergedData := componentConfig.MergeWithCLIData(repoURL, cliData)

		err := copier.ExecAdd(repoURL, mergedData)
		if err != nil {
			log.Error(err)
			os.Exit(1)

			return nil
		}

		fmt.Printf("Component %s added.\n", repoURL)
	}

	return nil
}

func Cmd() *cobra.Command {
	names := strings.Join(copier.EnabledShortNames, ", ")

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("add [%s or component_url]", names),
		Short:   "Add a component.",
		PreRunE: PreRunE,
		RunE:    RunE,
	}

	cmd.Flags().StringArrayVarP(&addFlags.DataArgs, "data", "d", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	cmd.Flags().StringVar(&addFlags.DataFile, "data-file", "", "Path to YAML file with default answers (follows copier data_file semantics)")

	return cmd
}
