// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package add

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/spf13/cobra"
)

func RunE(_ *cobra.Command, args []string) error {
	if !repo.IsInRepoRoot() {
		log.Error("Should be in repository root directory")
		os.Exit(1)
	}

	repoURL := args[0]

	fmt.Printf("Adding component %s\n", repoURL)

	err := copier.ExecAdd(repoURL)
	if err != nil {
		log.Error(err)

		// Prevent printing usage instructions on child command error
		return nil
	}

	fmt.Printf("Component %s added\n", repoURL)

	compose.Run(nil, nil)

	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add component",
		RunE:  RunE,
	}

	return cmd
}
