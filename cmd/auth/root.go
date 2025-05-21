// Copyright {{current_year}} DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"os"

	"github.com/spf13/cobra"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "DataRobot authentication commands",
	Long:  `Authentication commands for DataRobot CLI.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := AuthCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
