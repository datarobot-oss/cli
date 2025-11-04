// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/component/list"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, args []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("should be in repository root directory")
	}

	if len(args) == 0 || args[0] == "" {
		_ = list.RunE(nil, nil)

		return errors.New("answers_file required")
	}

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	yamlFile := args[0]

	if !isYamlFile(yamlFile) {
		answers, err := copier.AnswersFromPath(".")
		if err != nil {
			return err
		}

		answerMatches := make([]string, 0, len(answers))

		for _, answer := range answers {
			if strings.Contains(answer.FileName, yamlFile) {
				answerMatches = append(answerMatches, answer.FileName)
			}
		}

		if len(answerMatches) != 1 {
			_ = list.RunE(nil, nil)

			fmt.Println()

			return fmt.Errorf("answers_file that matches %s not found", yamlFile)
		}

		yamlFile = answerMatches[0]
	}

	fmt.Printf("Updating component %s\n", yamlFile)

	err := copier.ExecUpdate(yamlFile)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			log.Error("uv is not installed")
			os.Exit(1)

			return nil
		}

		log.Error(err)
		os.Exit(1)

		return nil
	}

	fmt.Printf("Component %s updated\n", yamlFile)

	compose.Run(nil, nil)

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update answers_file",
		Short:   "Update component",
		PreRunE: PreRunE,
		RunE:    RunE,
	}

	return cmd
}

func isYamlFile(yamlFile string) bool {
	info, err := os.Stat(yamlFile)

	if errors.Is(err, os.ErrNotExist) || info.IsDir() {
		return false
	}

	return strings.HasSuffix(yamlFile, ".yaml") || strings.HasSuffix(yamlFile, ".yml")
}
