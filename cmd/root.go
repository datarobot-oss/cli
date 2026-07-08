// Copyright 2026 DataRobot, Inc. and its affiliates.
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
	"github.com/datarobot/cli/cmd/artifact"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/cmd/component"
	"github.com/datarobot/cli/cmd/dependencies"
	"github.com/datarobot/cli/cmd/dotenv"
	llmgateway "github.com/datarobot/cli/cmd/llm-gateway"
	"github.com/datarobot/cli/cmd/pipeline"
	"github.com/datarobot/cli/cmd/plugin"
	"github.com/datarobot/cli/cmd/self"
	"github.com/datarobot/cli/cmd/start"
	"github.com/datarobot/cli/cmd/task"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/datarobot/cli/cmd/templates"
	"github.com/datarobot/cli/cmd/workload"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/telemetry"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

var (
	configFilePath   string
	rootOutputFormat outputformat.OutputFormat
)

// telemetryClient holds the active client for the current process. It is set
// in PersistentPreRunE so that cmd.Exit can flush events when main's error
// path fires (where only the signal context, not the cobra context, is available).
var telemetryClient *telemetry.Client

// RootCmd represents the base command when called without any subcommands.
// It uses CommandAdder to intelligently filter child commands based on feature gates.
var RootCmd = &cli.CommandAdder{
	Command: &cobra.Command{
		Use:     internalVersion.CliName,
		Version: internalVersion.Version,
		Short:   "Build AI Applications Faster",
		// TraverseChildren causes cobra to parse persistent flags left-to-right
		// before descending into each subcommand. This lets universal flags such
		// as --debug be placed before a plugin name (e.g. "dr --debug myplugin")
		// and still be consumed by core. Plugin commands set DisableFlagParsing:true
		// so args appearing AFTER the plugin name are never seen by core —
		// preserving the kubectl/helm "core stays blind to plugin args" model.
		TraverseChildren: true,
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// PersistentPreRunE is a hook called after flags are parsed
			// but before the command is run. Any logic that needs to happen
			// before ANY command execution should go here.
			log.Start()

			cmd.SilenceUsage = true // don’t spam usage for runtime errors

			err := initializeConfig(cmd)
			if err != nil {
				return err
			}

			if err := setupTLS(cmd); err != nil {
				return err
			}

			// Initialize telemetry client
			// Always collect common properties for logging (even in dry-run mode),
			// but only send to Amplitude if enabled.
			props := telemetry.CollectCommonProperties()

			// Stamp the command_kind common property based on whether
			// the dispatched command was registered via TrackPlugin.
			// CommonProperties is held by pointer inside Client, so this
			// late-bound update is visible at Track time.
			if props != nil {
				if telemetry.IsPluginCommand(cmd) {
					props.CommandKind = "plugin"
				} else {
					props.CommandKind = "core"
				}
			}
			// Log the detected shell only when debug is active. Reuse Shell from
			// telemetry props (already collected above) when available to avoid
			// spawning a redundant ps(1) subprocess on macOS.
			if log.GetLevel() <= log.DebugLevel {
				var shell string

				if props != nil {
					shell = props.Shell
				} else {
					shell = telemetry.DetectShell()
				}

				log.Debug("Shell", "name", shell)
			}

			client := telemetry.NewClient(props)

			// Store as process-level client so cmd.Exit can flush on the main error path.
			telemetryClient = client

			// Store telemetry client in context for use by commands
			cmd.SetContext(context.WithValue(cmd.Context(), telemetry.ClientContextKey{}, client))

			cobra.OnFinalize(func() {
				if event, ok := telemetry.EventFor(cmd, args); ok {
					client.Track(event)
				}

				client.Flush(3 * time.Second)

				log.Stop()
			})

			config.SetAPIConsumerTrace(config.CommandPathToTrace(cmd.CommandPath()))

			showFirstRunAnimation()

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			// Flush telemetry events before exit
			if client, ok := cmd.Context().Value(telemetry.ClientContextKey{}).(*telemetry.Client); ok {
				client.Flush(3 * time.Second)
			}

			log.Stop()

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	},
}

// ExecuteContext executes the root command with the given context.
// It adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func ExecuteContext(ctx context.Context) error {
	return RootCmd.ExecuteContext(ctx)
}

// bindUniversal binds name to viper and annotates the flag for forwarding to
// plugin subprocesses as DATAROBOT_CLI_<NAME>. The suffix is derived from the
// flag name (uppercased, hyphens → underscores). This is the single
// registration point — adding a new universal flag only requires one call here.
func bindUniversal(name string) {
	flag := RootCmd.PersistentFlags().Lookup(name)
	_ = viperx.BindPFlag(name, flag)

	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}

	flag.Annotations[config.UniversalAnnotationKey] = []string{strings.ToUpper(strings.ReplaceAll(name, "-", "_"))}
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
	RootCmd.PersistentFlags().Duration("plugin-discovery-timeout", 2*time.Second, "timeout for plugin discovery (0s disables)")
	RootCmd.PersistentFlags().Duration("plugin-update-check-interval", internalPlugin.DefaultUpdateCheckInterval, "cooldown between plugin update checks (0s disables)")
	RootCmd.PersistentFlags().Bool("skip-plugin-update-check", false, "skip plugin update checks before running plugins")
	RootCmd.PersistentFlags().Bool("disable-telemetry", false, "disable anonymous usage telemetry")

	// Private CA / TLS flags
	RootCmd.PersistentFlags().BoolP("skip-certificate-check", "k", false, "skip TLS certificate verification (insecure)")
	RootCmd.PersistentFlags().String("ca-cert", "", "path to a PEM-encoded CA certificate bundle")
	registerExportWindowsCertsFlag(RootCmd.Command)

	outputformat.AddPersistentFlag(RootCmd.Command, &rootOutputFormat)

	// Universal flags: bound to viper AND forwarded to plugin subprocesses as DATAROBOT_CLI_* env vars.
	// To add a new universal flag, call bindUniversal here next to its registration above.
	bindUniversal("debug")
	bindUniversal("disable-telemetry")
	bindUniversal("verbose")
	bindUniversal("skip-certificate-check")
	bindUniversal("ca-cert")

	// Non-universal flags: bound to viper only.
	_ = viperx.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	_ = viperx.BindPFlag("skip-auth", RootCmd.PersistentFlags().Lookup("skip-auth"))
	_ = viperx.BindPFlag("force-interactive", RootCmd.PersistentFlags().Lookup("force-interactive"))
	_ = viperx.BindPFlag("plugin-discovery-timeout", RootCmd.PersistentFlags().Lookup("plugin-discovery-timeout"))
	_ = viperx.BindPFlag("plugin-update-check-interval", RootCmd.PersistentFlags().Lookup("plugin-update-check-interval"))
	_ = viperx.BindPFlag("skip-plugin-update-check", RootCmd.PersistentFlags().Lookup("skip-plugin-update-check"))
	_ = viperx.BindPFlag("output-format", RootCmd.PersistentFlags().Lookup("output-format"))

	// Add command groups (plugin group added conditionally by registerPluginCommands)
	RootCmd.AddGroup(
		&cobra.Group{ID: "core", Title: tui.BaseTextStyle.Render("Core Commands:")},
		&cobra.Group{ID: "self", Title: tui.BaseTextStyle.Render("Self Commands:")},
		&cobra.Group{ID: "advanced", Title: tui.BaseTextStyle.Render("Advanced Commands:")},
	)

	// Add commands here to ensure that they are available to users.
	// Be sure to set the command's GroupID field appropriately;
	// otherwise the command will be added under 'Additional Commands'.
	// Commands with disabled feature gates are automatically filtered by cli.CommandAdder.
	RootCmd.AddCommand(
		artifact.Cmd(),
		auth.Cmd(),
		component.Cmd(),
		dependencies.Cmd(),
		dotenv.Cmd(),
		llmgateway.Cmd(),
		run.Cmd(),
		self.Cmd(),
		start.Cmd(),
		task.Cmd(),
		templates.Cmd(),
		workload.Cmd(),
		plugin.Cmd(),
		pipeline.Cmd(),
	)

	// Discover and register plugin commands
	plugin.RegisterPluginCommands(RootCmd.Command)

	// Guard parent commands against unrecognised positional args.
	// Must run after all commands (including plugins) are registered.
	setUnknownArgGuards(RootCmd.Command)

	// Override the default help command to add --all-commands flag
	defaultHelpFunc := RootCmd.HelpFunc()

	RootCmd.SetUsageTemplate(CustomUsageTemplate)

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

// setUnknownArgGuards walks the command tree and installs a RunE + Args validator
// on every pure-parent command (has subcommands, no Run/RunE, no explicit Args set).
//
// Cobra checks !Runnable() before ValidateArgs, so a non-runnable parent silently
// shows help for any arg. Making the command runnable lets Args fire first and
// return a clear error; the RunE fallback shows help when no args are given.
func setUnknownArgGuards(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		setUnknownArgGuards(cmd)
	}

	if !root.HasSubCommands() {
		return
	}

	if root.Args != nil || root.RunE != nil || root.Run != nil {
		return
	}

	root.Args = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}

		return fmt.Errorf("unknown command: %s", args[0])
	}

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
}

// initializeConfig initializes the configuration by reading from
// various sources such as environment variables and config files.
func initializeConfig(_ *cobra.Command) error {
	var err error

	// Set up Viper to process environment variables
	// First automatically map any environment variables
	// that are prefixed with DATAROBOT_CLI_ to config keys
	viperx.SetEnvPrefix("DATAROBOT_CLI")
	viperx.AutomaticEnv()
	viperx.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// map VISUAL and EDITOR to external-editor config key,
	// but set a default value
	viperx.SetDefault("external-editor", "vi")

	_ = viperx.BindEnv("external-editor", "VISUAL", "EDITOR")

	// API consumer tracking is enabled by default.
	// Set DATAROBOT_API_CONSUMER_TRACKING_ENABLED=false to opt out,
	// matching the Python SDK convention.
	viperx.SetDefault(config.APIConsumerTrackingEnabled, true)

	_ = viperx.BindEnv(config.APIConsumerTrackingEnabled, "DATAROBOT_API_CONSUMER_TRACKING_ENABLED")

	// If DATAROBOT_CLI_CONFIG is set and no explicit --config flag was provided,
	// use the environment variable value
	if configFilePath == "" {
		if envConfigPath := viperx.GetString("config"); envConfigPath != "" {
			configFilePath = envConfigPath
		}
	}

	// Now read the config file
	err = config.ReadConfigFile(configFilePath)
	if err != nil {
		return fmt.Errorf("Failed to read config file: %w", err)
	}

	return nil
}

// showFirstRunAnimation displays the animated DataRobot logo inline (no alt-screen)
// on the very first CLI invocation, so the final settled frame remains in normal
// terminal scrollback and whatever runs next (help text, or dr start's own inline
// wizard) continues directly below it instead of hard-cutting from a full-screen
// takeover. It checks global state to avoid showing it more than once, only runs
// in interactive terminals, and is skipped entirely under automation (e.g. tools
// like expect that attach a real pty and would otherwise see animation frames
// mixed into output they're pattern-matching).
func showFirstRunAnimation() {
	if reader.IsNonInteractive() {
		return
	}

	if !state.IsFirstRun() {
		return
	}

	// Only show animation when stdout is a terminal (not piped or redirected)
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return
	}

	m := tui.NewLogoAnimationModel()

	_, _ = tui.Run(m)

	state.MarkAnimationShown()
}
