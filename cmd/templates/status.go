// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package templates

import (
	"fmt"
	"os/exec"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the DataRobot application",
	Long:  `Check the status of the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Checking the status of the DataRobot application...")
		gitcmd := exec.Command("git", "status")
		stdout, err := gitcmd.Output()
		if err != nil {
			log.Fatal(err)
			return
		}

		// Print the output
		fmt.Println(string(stdout))
	},
}
