// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/allcommands"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/component"
	"github.com/datarobot/cli/cmd/dependencies"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/plugin"
	"github.com/datarobot/cli/cmd/self"
	"github.com/datarobot/cli/cmd/start"
	"github.com/datarobot/cli/cmd/task"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configFilePath string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:     internalVersion.CliName,
	Version: internalVersion.Version,
	Short:   "Build AI Applications Faster",
	Long: `
The DataRobot CLI helps you quickly set up, configure, and deploy AI applications
using pre-built templates. Get from idea to production in minutes, not hours.

✨ ` + tui.BaseTextStyle.Render("What you can do:") + `
  • Choose from ready-made AI application templates
  • Set up your development environment quickly
  • Deploy to DataRobot with a single command
  • Manage environment variables and configurations

🎯 ` + tui.BaseTextStyle.Render("Quick Start:") + `
  dr start             # Create your first AI app (start here!)
  dr --help            # Show all available commands

💡 ` + tui.BaseTextStyle.Render("New to AI development?") + ` Perfect! Run 'dr start' and we'll guide you through everything.`,
	// Show help by default when no subcommands match
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// PersistentPreRunE is a hook called after flags are parsed
		// but before the command is run. Any logic that needs to happen
		// before ANY command execution should go here.

		log.Start()

		return initializeConfig(cmd)
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		log.Stop()
	},
}

// ExecuteContext executes the root command with the given context.
// It adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func ExecuteContext(ctx context.Context) error {
	return RootCmd.ExecuteContext(ctx)
}

func init() {
	// Allow invoking commands in a case-insensitive manner
	cobra.EnableCaseInsensitive = true

	// Disable Cobra's default completion command since we have our own under 'self'
	RootCmd.CompletionOptions.DisableDefaultCmd = true

	// Set custom version template to match our unified format
	RootCmd.SetVersionTemplate(internalVersion.GetAppNameVersionText() + "\n")

	// Configure persistent flags
	RootCmd.PersistentFlags().StringVar(&configFilePath, "config", "",
		"path to config file (default location: $HOME/.config/datarobot/drconfig.yaml)")
	RootCmd.PersistentFlags().BoolP("version", "V", false, "display the version")
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	RootCmd.PersistentFlags().Bool("debug", false, "debug output")
	RootCmd.PersistentFlags().Bool("all-commands", false, "display all available commands and their flags in tree format")
	RootCmd.PersistentFlags().Bool("skip-auth", false, "skip authentication checks (for advanced users)")
	RootCmd.PersistentFlags().Bool("force-interactive", false, "force setup wizards to run even if already completed")
	RootCmd.PersistentFlags().Duration("plugin-discovery-timeout", 3*time.Second, "timeout for plugin discovery (0s disables)")

	// Make some of these flags available via Viper
	_ = viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	_ = viper.BindPFlag("verbose", RootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("skip-auth", RootCmd.PersistentFlags().Lookup("skip-auth"))
	_ = viper.BindPFlag("force-interactive", RootCmd.PersistentFlags().Lookup("force-interactive"))
	_ = viper.BindPFlag("plugin-discovery-timeout", RootCmd.PersistentFlags().Lookup("plugin-discovery-timeout"))

	// Add command groups (plugin group added conditionally by registerPluginCommands)
	RootCmd.AddGroup(
		&cobra.Group{ID: "core", Title: tui.BaseTextStyle.Render("Core Commands:")},
		&cobra.Group{ID: "self", Title: tui.BaseTextStyle.Render("Self Commands:")},
		&cobra.Group{ID: "advanced", Title: tui.BaseTextStyle.Render("Advanced Commands:")},
	)

	// Add commands here to ensure that they are available to users.
	// Be sure to set the command's GroupID field appropriately;
	// otherwise the command will be added under 'Additional Commands'.
	RootCmd.AddCommand(
		auth.Cmd(),
		component.Cmd(),
		dependencies.Cmd(),
		dotenv.Cmd(),
		run.Cmd(),
		self.Cmd(),
		start.Cmd(),
		task.Cmd(),
		templates.Cmd(),
		plugin.Cmd(),
	)

	// Discover and register plugin commands
	registerPluginCommands()

	// Override the default help command to add --all-commands flag
	defaultHelpFunc := RootCmd.HelpFunc()

	RootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		showAllCommands, _ := cmd.Flags().GetBool("all-commands")
		showVersion, _ := cmd.Flags().GetBool("version")

		if showAllCommands {
			output := allcommands.GenerateCommandTree(cmd.Root())

			_, _ = fmt.Fprint(cmd.OutOrStdout(), output)
		} else if showVersion {
			fmt.Fprintln(cmd.OutOrStdout(), internalVersion.GetAppNameVersionText())
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
	var err error

	// Set up Viper to process environment variables
	// First automatically map any environment variables
	// that are prefixed with DATAROBOT_CLI_ to config keys
	viper.SetEnvPrefix("DATAROBOT_CLI")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// map VISUAL and EDITOR to external-editor config key,
	// but set a default value
	viper.SetDefault("external-editor", "vi")

	_ = viper.BindEnv("external-editor", "VISUAL", "EDITOR")

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

// registerPluginCommands discovers and registers plugin commands
func registerPluginCommands() {
	timeout := viper.GetDuration("plugin-discovery-timeout")
	if timeout <= 0 {
		log.Debug("Plugin discovery disabled", "timeout", timeout)
		return
	}

	// Get list of builtin command names FIRST (before adding plugins)
	builtinNames := make(map[string]bool)
	for _, cmd := range RootCmd.Commands() {
		builtinNames[cmd.Name()] = true
	}

	type pluginDiscoveryResult struct {
		plugins []internalPlugin.DiscoveredPlugin
		err     error
	}

	resultCh := make(chan pluginDiscoveryResult, 1)

	go func() {
		plugins, err := internalPlugin.GetPlugins()
		resultCh <- pluginDiscoveryResult{plugins: plugins, err: err}
	}()

	var plugins []internalPlugin.DiscoveredPlugin

	select {
	case r := <-resultCh:
		if r.err != nil {
			log.Debug("Plugin discovery failed", "error", r.err)
			return
		}

		plugins = r.plugins
	case <-time.After(timeout):
		log.Info("Plugin discovery timed out", "timeout", timeout)
		log.Info("Consider increasing timeout using --plugin-discovery-timeout flag")

		return
	}

	if len(plugins) == 0 {
		// No plugins found, don't add empty group header
		return
	}

	// Only add plugin group if we have plugins to show
	RootCmd.AddGroup(&cobra.Group{
		ID:    "plugin",
		Title: tui.BaseTextStyle.Render("Plugin Commands:"),
	})

	for _, p := range plugins {
		// Skip if conflicts with builtin command
		if builtinNames[p.Manifest.Name] {
			// TODO: Consider logging at Info level since this affects user-visible behavior
			log.Debug("Plugin name conflicts with builtin command",
				"plugin", p.Manifest.Name,
				"path", p.Executable)

			continue
		}

		RootCmd.AddCommand(createPluginCommand(p))
	}
}

func createPluginCommand(p internalPlugin.DiscoveredPlugin) *cobra.Command {
	executable := p.Executable // Capture for closure
	manifest := p.Manifest     // Capture for closure
	pluginName := p.Manifest.Name

	return &cobra.Command{
		Use:                p.Manifest.Name,
		Short:              p.Manifest.Description,
		GroupID:            "plugin",
		DisableFlagParsing: true, // Pass all args to plugin
		DisableSuggestions: true,
		Run: func(_ *cobra.Command, args []string) {
			fmt.Println(tui.InfoStyle.Render("🔌 Running plugin: " + pluginName))
			log.Debug("Executing plugin", "name", pluginName, "executable", executable)

			exitCode := internalPlugin.ExecutePlugin(manifest, executable, args)
			os.Exit(exitCode)
		},
	}
}
