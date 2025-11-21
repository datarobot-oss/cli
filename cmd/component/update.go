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

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	if setupModel, ok := finalModel.(tui.InterruptibleModel); ok {
		if innerModel, ok := setupModel.Model.(Model); ok {
			fmt.Println(innerModel.exitMessage)
		}
	}

	return nil
}

var (
	recopy bool
	quiet  bool
)

func UpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [answers_file]",
		Short:   "Update installed component.",
		PreRunE: UpdatePreRunE,
		RunE:    UpdateRunE,
	}

	cmd.Flags().BoolVarP(&recopy, "recopy", "r", false, "Regenerate an existing component with different answers.")
	cmd.Flags().BoolVarP(&recopy, "quiet", "q", false, "Suppress status output.")

	return cmd
}

func runUpdate(yamlFile string) error {
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

	execErr := copier.ExecUpdate(yamlFile, recopy, quiet, debug)
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
