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
	"github.com/datarobot/cli/cmd/allcommands"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/completion"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/task"
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

// ExecuteContext executes the root command with the given context.
func ExecuteContext(ctx context.Context) error {
	return RootCmd.ExecuteContext(ctx)
}

func init() {
	cobra.OnInitialize(initConfig)

	err := config.ReadConfigFile("")
	if err != nil {
		log.Fatal(err)
	}

	// Add command groups
	RootCmd.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "advanced", Title: "Advanced Commands:"},
		&cobra.Group{ID: "plugin", Title: "Plugin Commands:"},
	)

	// Add commands here to ensure that they are available here.
	// Be sure to set the command's GroupID field appropriately;
	// otherwise the command will be added under 'Additional Commands'.
	RootCmd.AddCommand(
		auth.Cmd(),
		completion.Cmd(),
		dotenv.Cmd(),
		task.Cmd(),
		templates.Cmd(),
		version.Cmd(),
	)

	// Override the default help command to add --all-commands flag
	defaultHelpFunc := RootCmd.HelpFunc()

	RootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		showAllCommands, _ := cmd.Flags().GetBool("all-commands")

		if showAllCommands {
			output := allcommands.GenerateCommandTree(cmd.Root())

			_, _ = fmt.Fprint(cmd.OutOrStdout(), output)
		} else {
			// Use default help behavior
			defaultHelpFunc(cmd, args)
		}
	})

	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	RootCmd.PersistentFlags().Bool("all-commands", false, "display all available commands and their flags in tree format")
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
