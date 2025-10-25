// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"context"
	"fmt"

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

var configFilePath string

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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// PersistentPreRunE is a hook called after flags are parsed
		// but before the command is run. Any logic that needs to happen
		// before ANY command execution should go here.
		return initializeConfig(cmd)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return RootCmd.Execute()
}

// ExecuteContext executes the root command with the given context.
func ExecuteContext(ctx context.Context) error {
	return RootCmd.ExecuteContext(ctx)
}

func init() {
	// Configure persistent flags
	RootCmd.PersistentFlags().StringVar(&configFilePath, "config", "",
	"path to config file (default location: $HOME/.datarobot/drconfig.yaml)")
	// verbose and debug are special
	// flags that
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	_ = viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))

	setLogLevel()

	RootCmd.AddCommand(
		auth.Cmd(),
		completion.Cmd(),
		dotenv.Cmd(),
		run.Cmd(),
		templates.Cmd(),
		version.Cmd(),
	)

}

func initializeConfig(cmd *cobra.Command) error {
	// Set up Viper to process environment variables
	viper.SetEnvPrefix("DATAROBOT")
	// Automatically map environment variables that are prefixed
	// DATAROBOT_ to config keys
	viper.AutomaticEnv()

	// Also map USE_USE_DATAROBOT_LLM_GATEWAY
	viper.BindEnv("use_datarobot_llm_gateway", "USE_DATAROBOT_LLM_GATEWAY")

	config.ReadConfigFile(configFilePath)

	// Bind Cobra flags to Viper
	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
	}

	fmt.Println("Configuration initialized. Using config file:", viper.ConfigFileUsed())
	// Print out the viper configuration for debugging
	for key, value := range viper.AllSettings() {
		// TODO Skip token because its sensitive
		if key == "token" {
			fmt.Printf("  %s: %s\n", key, "****")
		} else {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	return nil
}

func setLogLevel() {
	if viper.GetBool("debug") {
		log.SetLevel(log.DebugLevel)
	} else if viper.GetBool("verbose") {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}
}
