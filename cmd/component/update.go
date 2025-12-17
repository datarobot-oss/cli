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
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func UpdatePreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	return nil
}

func UpdateRunE(cmd *cobra.Command, args []string) error { //nolint: cyclop
	if viper.GetBool("debug") {
		f, err := tea.LogToFile("tea-debug.log", "debug")
		if err != nil {
			fmt.Println("fatal: ", err)
			os.Exit(1)
		}

		defer f.Close()
	}

	var updateFileName string
	if len(args) > 0 && args[0] != "" {
		updateFileName = args[0]
	}

	cliData, err := parseDataArgs(dataArgs)
	if err != nil {
		fmt.Println("Fatal:", err)
		os.Exit(1)
	}

	// If file name has been provided
	if updateFileName != "" {
		err := runUpdate(updateFileName, cliData, dataFile)
		if err != nil {
			fmt.Println("Fatal:", err)
			os.Exit(1)
		}

		compose.Cmd().Run(nil, nil)
		run.Cmd().Run(nil, []string{"reinstall"})

		return nil
	}

	m := NewUpdateComponentModel()
	p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	if setupModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := setupModel.Model.(Model); ok {
			fmt.Println(innerModel.exitMessage)

			if innerModel.componentUpdated {
				compose.Cmd().Run(nil, nil)
				run.Cmd().Run(nil, []string{"reinstall"})
			}
		}
	}

	return nil
}

var (
	dataArgs  []string
	dataFile  string
	recopy    bool
	quiet     bool
	overwrite bool
)

func UpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [answers_file]",
		Short:   "Update installed component.",
		PreRunE: UpdatePreRunE,
		RunE:    UpdateRunE,
	}

	cmd.Flags().StringArrayVar(&dataArgs, "data", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "Path to YAML file with default answers (follows copier data_file semantics)")
	cmd.Flags().BoolVarP(&recopy, "recopy", "r", false, "Regenerate an existing component with different answers.")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress status output.")
	cmd.Flags().BoolVarP(&overwrite, "overwrite", "w", false, "Overwrite files even if they exist.")

	return cmd
}

func runUpdate(yamlFile string, cliData map[string]interface{}, dataFilePath string) error {
	// Clean path like this `./.datarobot/answers/cli/../react-frontend_web.yml`
	// to .datarobot/answers/react-frontend_web.yml
	yamlFile = filepath.Clean(yamlFile)

	if !isYamlFile(yamlFile) {
		return errors.New("The supplied file is not a YAML file.")
	}

	answers, err := copier.AnswersFromPath(".", false)
	if err != nil {
		return err
	}

	answersContainFile := slices.ContainsFunc(answers, func(answer copier.Answers) bool {
		return answer.FileName == yamlFile
	})

	if !answersContainFile {
		return errors.New("The supplied filename doesn't exist in answers.")
	}

	debug := viper.GetBool("debug")

	// Get the repo URL from the answers file to look up defaults
	repoURL, err := getRepoURLFromAnswersFile(yamlFile)
	if err != nil {
		return err
	}

	// Load component defaults configuration
	componentConfig, err := config.LoadComponentDefaults(dataFilePath)
	if err != nil {
		log.Warn("Failed to load component defaults", "error", err)

		componentConfig = &config.ComponentDefaults{
			Defaults: make(map[string]map[string]interface{}),
		}
	}

	// Merge defaults with CLI data (CLI data takes precedence)
	mergedData := componentConfig.MergeWithCLIData(repoURL, cliData)

	execErr := copier.ExecUpdate(yamlFile, mergedData, recopy, quiet, debug, overwrite)
	if execErr != nil {
		// TODO: Check beforehand if uv is installed or not
		if errors.Is(execErr, exec.ErrNotFound) {
			log.Error("uv is not installed.")
		}

		return execErr
	}

	return nil
}

// getRepoURLFromAnswersFile reads the _src_path from a copier answers file
func getRepoURLFromAnswersFile(yamlFile string) (string, error) {
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return "", fmt.Errorf("failed to read answers file: %w", err)
	}

	var answers struct {
		SrcPath string `yaml:"_src_path"`
	}

	if err := yaml.Unmarshal(data, &answers); err != nil {
		return "", fmt.Errorf("failed to parse answers file: %w", err)
	}

	if answers.SrcPath == "" {
		return "", errors.New("answers file missing _src_path field")
	}

	return answers.SrcPath, nil
}

// TODO: Maybe use `IsValidYAML` from /internal/misc/yaml/validation.go instead or even move this function there
func isYamlFile(yamlFile string) bool {
	info, err := os.Stat(yamlFile)

	if errors.Is(err, os.ErrNotExist) || info.IsDir() {
		return false
	}

	return strings.HasSuffix(yamlFile, ".yaml") || strings.HasSuffix(yamlFile, ".yml")
}
