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
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/allcommands"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/component"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/self"
	"github.com/datarobot/cli/cmd/start"
	"github.com/datarobot/cli/cmd/task"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/internal/config"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configFilePath string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   internalVersion.CliName,
	Short: "🚀 " + internalVersion.AppName + " - Build AI Applications Faster",
	Long: `
The DataRobot CLI helps you quickly set up, configure, and deploy AI applications
using pre-built templates. Get from idea to production in minutes, not hours.

✨ ` + tui.BaseTextStyle.Render("What you can do:") + `
  • Choose from ready-made AI application templates
  • Set up your development environment quickly
  • Deploy to DataRobot with a single command
  • Manage environment variables and configurations

🎯 ` + tui.BaseTextStyle.Render("Quick Start:") + `
  dr templates setup   # Interactive setup wizard
  dr run dev           # Start development server
  dr --help            # Show all available commands

💡 ` + tui.BaseTextStyle.Render("New to DataRobot CLI?") + ` Run 'dr templates setup' to get started!`,
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
	// Allow invoking commands in a case-insensitive manner
	cobra.EnableCaseInsensitive = true

	// Disable Cobra's default completion command since we have our own under 'self'
	RootCmd.CompletionOptions.DisableDefaultCmd = true

	// Configure persistent flags
	RootCmd.PersistentFlags().StringVar(&configFilePath, "config", "",
		"path to config file (default location: $HOME/.datarobot/drconfig.yaml)")
	RootCmd.PersistentFlags().BoolP("version", "V", false, "display the version")
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	RootCmd.PersistentFlags().Bool("all-commands", false, "display all available commands and their flags in tree format")
	RootCmd.PersistentFlags().Bool("skip-auth", false, "skip authentication checks (for advanced users)")
	RootCmd.PersistentFlags().Bool("force-interactive", false, "force setup wizards to run even if already completed")

	// Make some of these flags available via Viper
	_ = viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("skip-auth", RootCmd.PersistentFlags().Lookup("skip-auth"))
	_ = viper.BindPFlag("force-interactive", RootCmd.PersistentFlags().Lookup("force-interactive"))

	setLogLevel()

	// Add command groups
	RootCmd.AddGroup(
		&cobra.Group{ID: "core", Title: tui.BaseTextStyle.Render("Core Commands:")},
		&cobra.Group{ID: "self", Title: tui.BaseTextStyle.Render("Self Commands:")},
		&cobra.Group{ID: "advanced", Title: tui.BaseTextStyle.Render("Advanced Commands:")},
		&cobra.Group{ID: "plugin", Title: tui.BaseTextStyle.Render("Plugin Commands:")},
	)

	// Add commands here to ensure that they are available to users.
	// Be sure to set the command's GroupID field appropriately;
	// otherwise the command will be added under 'Additional Commands'.
	RootCmd.AddCommand(
		auth.Cmd(),
		component.Cmd(),
		dotenv.Cmd(),
		run.Cmd(),
		self.Cmd(),
		start.Cmd(),
		task.Cmd(),
		templates.Cmd(),
	)

	// Override the default help command to add --all-commands flag
	defaultHelpFunc := RootCmd.HelpFunc()

	RootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		showAllCommands, _ := cmd.Flags().GetBool("all-commands")
		showVersion, _ := cmd.Flags().GetBool("version")

		if showAllCommands {
			output := allcommands.GenerateCommandTree(cmd.Root())

			_, _ = fmt.Fprint(cmd.OutOrStdout(), output)
		} else if showVersion {
			fmt.Fprintln(cmd.OutOrStdout(), tui.BaseTextStyle.Render(internalVersion.AppName)+" (version "+tui.InfoStyle.Render(internalVersion.Version)+")")
		} else {
			// Use default help behavior but with customized template
			RootCmd.SetHelpTemplate(CustomHelpTemplate)
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
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Now map other environment variables to config keys
	// such as those used by the DataRobot platform or other SDKs
	// and clients. If the DATAROBOT_CLI equivalents are not set,
	// then Viper will fallback to these
	err := viper.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
	if err != nil {
		return fmt.Errorf("Failed to bind environment variables for endpoint: %w", err)
	}

	err = viper.BindEnv("token", "DATAROBOT_API_TOKEN")
	if err != nil {
		return fmt.Errorf("Failed to bind environment variables for token: %w", err)
	}

	// map VISUAL and EDITOR to external-editor config key
	err = viper.BindEnv("external-editor", "VISUAL", "EDITOR")
	if err != nil {
		return fmt.Errorf("Failed to bind environment variables for external-editor: %w", err)
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
		return fmt.Errorf("Failed to read config file: %w", err)
	}

	// Bind Cobra flags to Viper
	err = viper.BindPFlags(cmd.Flags())
	if err != nil {
		return err
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
