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
	"os/exec"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, args []string) error {
	if !repo.IsInRepoRoot() {
		return fmt.Errorf("should be in repository root directory")
	}

	if len(args) == 0 || args[0] == "" {
		return fmt.Errorf("component_url required")
	}

	return nil
}

func RunE(_ *cobra.Command, args []string) error {
	repoURL := args[0]

	fmt.Printf("Adding component %s\n", repoURL)

	err := copier.ExecAdd(repoURL)
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

	fmt.Printf("Component %s added\n", repoURL)

	compose.Run(nil, nil)

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add component_url",
		Short:   "Add component",
		PreRunE: PreRunE,
		RunE:    RunE,
	}

	return cmd
}
