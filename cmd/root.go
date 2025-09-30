// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/completion"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/run"
	"github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/cmd/version"
	"github.com/datarobot/cli/internal/config"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   internalVersion.CliName,
	Short: "The " + internalVersion.AppName,
	Long: `
	The ` + internalVersion.AppName + ` is a command-line interface for interacting with
	DataRobot's application templates and authentication. It allows users to 
	clone, configure, and deploy applications to their DataRobot production environment.
	`,
	// Show help by default when no subcommands match
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	err := config.ReadConfigFile("")
	if err != nil {
		log.Fatal(err)
	}

	RootCmd.AddCommand(
		auth.Cmd(),
		completion.Cmd(),
		dotenv.Cmd(),
		run.Cmd(),
		templates.Cmd(),
		version.Cmd(),
	)
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	_ = viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
}

func initConfig() {
	if viper.GetBool("debug") {
		log.SetLevel(log.DebugLevel)
	} else if viper.GetBool("verbose") {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
}
