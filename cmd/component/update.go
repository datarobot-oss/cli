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
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func UpdatePreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	return nil
}

func UpdateRunE(cmd *cobra.Command, args []string) error {
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

	// If file name has been provided
	if updateFileName != "" {
		err := runUpdate(updateFileName)
		if err != nil {
			fmt.Println("Fatal: ", err)
			os.Exit(1)
		}

		return nil
	}

	m := NewUpdateComponentModel()
	p := tea.NewProgram(tui.NewInterruptibleModel(m), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

func UpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update answers_file",
		Short:   "Update installed component.",
		PreRunE: UpdatePreRunE,
		RunE:    UpdateRunE,
	}

	return cmd
}

func runUpdate(yamlFile string) error {
	if !isYamlFile(yamlFile) {
		return errors.New("The supplied file is not a YAML file.")
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
		return errors.New("The supplied filename doesn't exist in answers.")
	}

	quiet := false
	debug := viper.GetBool("debug")
	recopy := false // TODO: fix this

	execErr := copier.ExecUpdate(yamlFile, quiet, debug, recopy)
	if execErr != nil {
		// TODO: Check beforehand if uv is installed or not
		if errors.Is(execErr, exec.ErrNotFound) {
			log.Error("uv is not installed.")
		}

		return execErr
	}

	return nil
}

// TODO: Maybe use `IsValidYAML` from /internal/misc/yaml/validation.go instead or even move this function there
func isYamlFile(yamlFile string) bool {
	info, err := os.Stat(yamlFile)

	if errors.Is(err, os.ErrNotExist) || info.IsDir() {
		return false
	}

	return strings.HasSuffix(yamlFile, ".yaml") || strings.HasSuffix(yamlFile, ".yml")
}
