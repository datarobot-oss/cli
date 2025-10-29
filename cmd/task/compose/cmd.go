// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package compose

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

func Run(_ *cobra.Command, _ []string) {
	taskfileName := "Taskfile.yaml"
	discovery := task.NewTaskDiscovery(taskfileName)

	taskFilePath, err := discovery.Discover(".", 2)
	if err != nil {
		task.ExitWithError(err)
		return
	}

	fmt.Printf("Generated file saved to: %s\n", taskFilePath)

	contentBytes, err := os.ReadFile(".gitignore")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Error(fmt.Errorf("failed to read from .gitignore file: %w", err))
		return
	}

	contents := string(contentBytes)

	if !strings.Contains(contents, taskfileName) {
		f, err := os.Create(".gitignore")
		if err != nil {
			log.Error(fmt.Errorf("failed to create .gitignore file: %w", err))
			return
		}

		defer f.Close()

		_, err = f.WriteString(taskfileName + "\n\n" + contents)
		if err != nil {
			log.Error(fmt.Errorf("failed to write to .gitignore file: %w", err))
			return
		}

		fmt.Printf("Added " + taskfileName + " file to .gitignore\n")
	}

	return
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compose",
		Short: "Compose Taskfile.yaml from multiple files in subdirectories",
		Run:   Run,
	}
}
