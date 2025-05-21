// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   version.AppName,
	Short: "The DataRobot CLI",
	Long: `
	The DataRobot CLI is a command-line interface for interacting with
	DataRobot's application templates and authentication. It allows users to 
	clone, configure, and deploy applications to their DataRobot production environment.
	`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	RootCmd.AddCommand(
		auth.AuthCmd,
		templates.TemplatesCmd,
		CompletionCmd,
		VersionCmd,
	)

	RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
