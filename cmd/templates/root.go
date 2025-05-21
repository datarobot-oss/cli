// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package templates

import (
	"os"

	"github.com/spf13/cobra"
)

var TemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "DataRobot application templates commands",
	Long:  `Application templates commands for DataRobot CLI.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := TemplatesCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
