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
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
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
		return errors.New("should be in repository root directory")
	}

	return nil
}

func UpdateRunE(cmd *cobra.Command, args []string) error {
	if err := setupDebugLogging(); err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}

	var updateFileName string
	if len(args) > 0 && args[0] != "" {
		updateFileName = args[0]
	}

	// User may provide CLI args --yes or -y or --interactive=false or -i=false in order to skip prompt
	yes, _ := cmd.Flags().GetBool("yes")
	interactive, _ := cmd.Flags().GetBool("interactive")

	doNotPrompt := yes || !interactive

	// Parse --data arguments
	dataArgs, _ := cmd.Flags().GetStringArray("data")

	cliData, err := parseDataArgs(dataArgs)
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}

	// Get --data-file path if specified
	dataFile, _ := cmd.Flags().GetString("data-file")

	// If we are skipping prompt and file name has been provided
	if doNotPrompt && updateFileName != "" {
		if err := runUpdateWithDataFile(updateFileName, cliData, dataFile); err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}

		return nil
	}

	// Currently if we using interactive mode we'll always go the list screen and pre-check a file if passed in args
	return runInteractiveUpdate(updateFileName)
}

func setupDebugLogging() error {
	if !viper.GetBool("debug") {
		return nil
	}

	f, err := tea.LogToFile("tea-debug.log", "debug")
	if err != nil {
		return err
	}

	defer f.Close()

	return nil
}

func runInteractiveUpdate(updateFileName string) error {
	m := NewUpdateComponentModel(updateFileName)
	p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

func UpdateCmd() *cobra.Command {
	var yes bool

	var interactive bool

	var dataArgs []string

	var dataFile string

	cmd := &cobra.Command{
		Use:     "update answers_file",
		Short:   "Update component",
		PreRunE: UpdatePreRunE,
		RunE:    UpdateRunE,
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Automatically confirm update without prompting")
	// TODO: Do we want to alter this to be interactive by default? Maybe once things are more ironed out.
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Set to false to automatically confirm update without prompting")
	cmd.Flags().StringArrayVar(&dataArgs, "data", []string{}, "Provide answer data in key=value format (can be specified multiple times)")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "Path to YAML file with default answers (follows copier data_file semantics)")

	return cmd
}

func runUpdateWithDataFile(yamlFile string, cliData map[string]interface{}, dataFilePath string) error {
	if !isYamlFile(yamlFile) {
		return errors.New("supplied file is not a yaml file")
	}

	answers, err := copier.AnswersFromPath(".")
	if err != nil {
		return err
	}

	answerFileNames := make([]string, 0, len(answers))

	for _, answer := range answers {
		answerFileNames = append(answerFileNames, answer.FileName)
	}

	// TODO: Account for consolidating on string representation
	// This check fails if I pass `./.datarobot/answers/react-frontend_web.yml` - which has the prefix of `./`
	if !slices.Contains(answerFileNames, yamlFile) {
		return errors.New("supplied filename doesn't exist in answers")
	}

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

	var execErr error
	if len(mergedData) > 0 {
		execErr = copier.ExecUpdateWithData(yamlFile, mergedData)
	} else {
		execErr = copier.ExecUpdate(yamlFile)
	}

	if execErr != nil {
		// TODO: Check beforehand if uv is installed or not
		if errors.Is(execErr, exec.ErrNotFound) {
			log.Error("uv is not installed")
		}

		return execErr
	}

	return nil
}

// parseDataArgs parses --data arguments in key=value format
func parseDataArgs(dataArgs []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, arg := range dataArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --data format: %s (expected key=value)", arg)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("empty key in --data argument: %s", arg)
		}

		// Try to parse boolean values
		if value == "true" {
			result[key] = true
			continue
		}

		if value == "false" {
			result[key] = false
			continue
		}

		// Otherwise store as string
		result[key] = value
	}

	return result, nil
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
