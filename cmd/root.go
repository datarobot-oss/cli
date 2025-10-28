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
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
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
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	RootCmd.PersistentFlags().Bool("all-commands", false, "display all available commands and their flags in tree format")
	// Make some of these flags available via Viper
	_ = viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))

	setLogLevel()

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
		run.Cmd(),
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
}

// initializeConfig initializes the configuration by reading from
// various sources such as environment variables and config files.
func initializeConfig(cmd *cobra.Command) error {
	// Set up Viper to process environment variables
	// First automatically map any environment variables
	// that are prefixed with DATAROBOT_CLI_ to config keys
	viper.SetEnvPrefix("DATAROBOT_CLI")
	viper.AutomaticEnv()

	// Now map other environment variables to config keys
	// such as those used by the DataRobot platform or other SDKs
	// and clients. If the DATAROBOT_CLI equivalents are not set,
	// then Viper will fallback to these
	err := viper.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
	if err != nil {
		return fmt.Errorf("failed to bind environment variables for endpoint: %w", err)
	}

	err = viper.BindEnv("api_token", "DATAROBOT_API_TOKEN")
	if err != nil {
		return fmt.Errorf("failed to bind environment variables for api_token: %w", err)
	}

	// map USE_DATAROBOT_LLM_GATEWAY
	err = viper.BindEnv("use_datarobot_llm_gateway", "USE_DATAROBOT_LLM_GATEWAY")
	if err != nil {
		return fmt.Errorf("failed to bind environment variables for use_datarobot_llm_gateway: %w", err)
	}

	// map VISUAL and EDITOR to external_editor config key
	err = viper.BindEnv("external_editor", "VISUAL", "EDITOR")
	if err != nil {
		return fmt.Errorf("failed to bind environment variables for external_editor: %w", err)
	}

	// If DATAROBOT_CLI_CONFIG is set and no explicit --config flag was provided,
	// use the environment variable value
	if configFilePath == "" {
		if envConfigPath := viper.GetString("config"); envConfigPath != "" {
			configFilePath = envConfigPath
		}
	}

	// Now read the config file
	err = config.ReadConfigFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Bind Cobra flags to Viper
	err = viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
	}

	// TODO Put this elsewhere

	// fmt.Println("Configuration initialized. Using config file:", viper.ConfigFileUsed())
	// // Print out the viper configuration for debugging
	// // Alphabetically, and redacting sensitive information
	// // TODO There has to be a better way of marking sensitive data
	// // perhaps with leebenson/conform?
	// keys := make([]string, 0, len(viper.AllSettings()))
	// for key := range viper.AllSettings() {
	// 	keys = append(keys, key)
	// }

	// sort.Strings(keys)

	// for _, key := range keys {
	// 	value := viper.Get(key)
	// 	// TODO Skip token because its sensitive
	// 	if key == "token" {
	// 		fmt.Printf("  %s: %s\n", key, "****")
	// 	} else {
	// 		fmt.Printf("  %s: %v\n", key, value)
	// 	}
	// }

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
