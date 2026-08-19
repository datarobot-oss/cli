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
	"strings"
	"time"

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
	"github.com/datarobot/cli/internal/outputformat"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/telemetry"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Dependencies
// ---------------------------------------------------------------------------

// ConfigInitializerFunc initializes viperx and reads the config file. It
// receives the cobra command being executed so flag values are accessible.
// Replace this in tests to skip disk I/O and prevent env-var bleed.
type ConfigInitializerFunc func(cmd *cobra.Command, configFilePath string) error

// TLSSetupFunc configures http.DefaultTransport based on TLS-related flags
// and the persisted ca-cert config value. Replace in tests to be a no-op.
type TLSSetupFunc func(cmd *cobra.Command) error

// TelemetryPropsFunc collects the common telemetry properties for an
// invocation. Replace in tests to return nil (disables telemetry).
type TelemetryPropsFunc func() *telemetry.CommonProperties

// TelemetryClientFunc builds a telemetry client from the collected properties.
// Replace in tests to return a no-op client.
type TelemetryClientFunc func(props *telemetry.CommonProperties) *telemetry.Client

// AnimationFunc is called once during the first CLI invocation to display the
// DataRobot logo animation. Replace in tests with a no-op.
type AnimationFunc func()

// PluginRegistrarFunc discovers and registers plugin subcommands on cmd.
// Replace in tests with a no-op.
type PluginRegistrarFunc func(cmd *cobra.Command)

// Dependencies bundles every injectable collaborator for the root command.
// The zero-value is not useful; use DefaultDependencies() or NewRootFactory()
// to get a properly populated instance.
type Dependencies struct {
	// ConfigInitializer initializes viperx and reads drconfig.yaml.
	ConfigInitializer ConfigInitializerFunc

	// TLSSetup configures the default HTTP transport for TLS.
	TLSSetup TLSSetupFunc

	// TelemetryProps collects common Amplitude properties once per invocation.
	TelemetryProps TelemetryPropsFunc

	// TelemetryClient builds the Amplitude client from collected properties.
	TelemetryClient TelemetryClientFunc

	// Animation runs the first-run logo animation.
	Animation AnimationFunc

	// PluginRegistrar discovers and wires plugin subcommands.
	PluginRegistrar PluginRegistrarFunc
}

// DefaultDependencies returns the production-ready set of dependencies
// wired to the real implementations used by the CLI binary.
func DefaultDependencies() Dependencies {
	return Dependencies{
		ConfigInitializer: defaultConfigInitializer,
		TLSSetup:          setupTLS,
		TelemetryProps:    telemetry.CollectCommonProperties,
		TelemetryClient:   telemetry.NewClient,
		Animation:         showFirstRunAnimation,
		PluginRegistrar:   plugin.RegisterPluginCommands,
	}
}

// ---------------------------------------------------------------------------
// Option helpers
// ---------------------------------------------------------------------------

// Option is a functional option applied to a RootFactory.
type Option func(*RootFactory)

// WithConfigInitializer overrides the config-initialization step. Useful in
// tests that need to skip disk reads or inject fixed viper state.
func WithConfigInitializer(fn ConfigInitializerFunc) Option {
	return func(f *RootFactory) {
		f.deps.ConfigInitializer = fn
	}
}

// WithTLSSetup overrides the TLS-setup step. Useful in tests that do not
// have a valid TLS environment.
func WithTLSSetup(fn TLSSetupFunc) Option {
	return func(f *RootFactory) {
		f.deps.TLSSetup = fn
	}
}

// WithTelemetryProps overrides the telemetry-properties collector. Pass a
// function that returns nil to disable all telemetry in a test.
func WithTelemetryProps(fn TelemetryPropsFunc) Option {
	return func(f *RootFactory) {
		f.deps.TelemetryProps = fn
	}
}

// WithTelemetryClient overrides the telemetry-client constructor. Useful in
// tests that want to capture or suppress telemetry events.
func WithTelemetryClient(fn TelemetryClientFunc) Option {
	return func(f *RootFactory) {
		f.deps.TelemetryClient = fn
	}
}

// WithAnimation overrides the first-run animation hook. Pass a no-op in tests.
func WithAnimation(fn AnimationFunc) Option {
	return func(f *RootFactory) {
		f.deps.Animation = fn
	}
}

// WithPluginRegistrar overrides the plugin-discovery hook. Pass a no-op in tests.
func WithPluginRegistrar(fn PluginRegistrarFunc) Option {
	return func(f *RootFactory) {
		f.deps.PluginRegistrar = fn
	}
}

// ---------------------------------------------------------------------------
// RootFactory
// ---------------------------------------------------------------------------

// RootFactory assembles the root cobra command from a set of injectable
// Dependencies. Each call to Build() produces a fresh, fully-wired
// *cli.CommandAdder, so multiple builds (e.g. in parallel tests) are
// independent of each other and of any package-level state.
//
// Use NewRootFactory(opts...) to construct one, or rely on the
// package-level RootCmd variable for the production singleton.
type RootFactory struct {
	// deps holds the injectable collaborators for this factory.
	deps Dependencies

	// configFilePath is the per-build config file override, populated from
	// the --config flag or DATAROBOT_CLI_CONFIG env var during Build().
	configFilePath string

	// telemetryClient holds the active client for the current process-level
	// build. It is also stored in the cobra context, but we keep a reference
	// here so Exit() can flush events when main's error path fires.
	telemetryClient *telemetry.Client
}

// NewRootFactory creates a RootFactory with the given functional options
// applied on top of DefaultDependencies(). This is the primary constructor
// for both production (zero options) and tests (injected overrides).
func NewRootFactory(opts ...Option) *RootFactory {
	f := &RootFactory{
		deps: DefaultDependencies(),
	}

	for _, o := range opts {
		o(f)
	}

	return f
}

// TelemetryClient returns the telemetry client that was wired during the
// most recent Build() call. cmd.Exit uses this to flush events on error
// paths where only the signal context, not a cobra context, is available.
func (f *RootFactory) TelemetryClient() *telemetry.Client {
	return f.telemetryClient
}

// Build constructs and returns a fully-wired *cli.CommandAdder. The returned
// command includes all registered sub-commands, persistent flags, help
// overrides, plugin discovery, and unknown-arg guards. Each call is
// self-contained: it does not mutate any package-level state.
func (f *RootFactory) Build() *cli.CommandAdder {
	// outputFormat is captured per-build (not package-level) so parallel
	// builds in tests cannot race on a shared variable.
	var outputFormat outputformat.OutputFormat

	cmd := f.buildRootCommand(&outputFormat)
	adder := &cli.CommandAdder{Command: cmd}

	f.registerFlags(adder, &outputFormat)
	f.bindViperFlags(adder)
	f.addGroups(adder)
	f.addSubcommands(adder)
	f.deps.PluginRegistrar(adder.Command)

	// Guard every pure-parent command against unrecognised positional args.
	// Must run after all sub-commands (including plugins) are registered.
	setUnknownArgGuards(adder.Command)

	f.configureHelp(adder)

	return adder
}

// buildRootCommand constructs the bare cobra.Command with its Use, Long, and
// PersistentPreRunE / PersistentPostRunE hooks wired to the factory's deps.
func (f *RootFactory) buildRootCommand(outputFormat *outputformat.OutputFormat) *cobra.Command {
	cmd := &cobra.Command{
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
  dr self update       # Update DataRobot CLI to latest version
  dr --help            # Show all available commands

💡 ` + tui.BaseTextStyle.Render("New to AI development?") + ` Perfect! Run 'dr start' and we'll guide you through everything.`,

		// PersistentPreRunE is called after flags are parsed but before the
		// command runs. All global initialization (logging, config, TLS,
		// telemetry) lives here so every sub-command inherits it automatically.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return f.persistentPreRun(cmd, args)
		},

		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			return f.persistentPostRun(cmd)
		},

		// RootCmd needs its own RunE (to show help on a bare "dr" invocation,
		// after the first-run animation), which disqualifies it from
		// setUnknownArgGuards' generic walk (that skips any command with a
		// pre-existing RunE). So it gets the same "unknown command" Args
		// validator explicitly here.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}

			return fmt.Errorf("unknown command: %s", args[0])
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Allow invoking commands in a case-insensitive manner.
	cobra.EnableCaseInsensitive = true

	// Disable Cobra's default completion command since we have our own under 'self'.
	cmd.CompletionOptions.DisableDefaultCmd = true

	// Set custom version template to match our unified format.
	cmd.SetVersionTemplate(internalVersion.GetAppNameVersionText() + "\n\nTo update: dr self update\n")

	// Only the prefix is styled; the message itself is often multi-line and is
	// the part a user copies into a bug report.
	cmd.SetErrPrefix(tui.ErrorStyle.Render("Error:"))

	return cmd
}

// persistentPreRun runs all global initialization in the correct order:
// logging, config, TLS, telemetry collection, and the first-run animation.
func (f *RootFactory) persistentPreRun(cmd *cobra.Command, args []string) error {
	log.Start()

	// Suppress cobra's usage printout for runtime errors — only show it for
	// flag-parsing failures, which happen before this hook runs.
	cmd.SilenceUsage = true

	// Read drconfig.yaml and bind env vars into viper.
	if err := f.deps.ConfigInitializer(cmd, f.configFilePath); err != nil {
		return err
	}

	// Configure the default HTTP transport (ca-cert, skip-verify).
	if err := f.deps.TLSSetup(cmd); err != nil {
		return err
	}

	// Collect common properties for telemetry (and optional debug logging).
	// Always collect even in dry-run mode; transmission is gated inside Client.
	props := f.deps.TelemetryProps()

	if props != nil {
		// Stamp command_kind so every event knows whether it came from a core
		// command or a plugin.
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

	client := f.deps.TelemetryClient(props)

	// Store as factory-level client so cmd.Exit can flush on the main error path.
	f.telemetryClient = client

	// Store telemetry client in context for use by sub-commands.
	cmd.SetContext(context.WithValue(cmd.Context(), telemetry.ClientContextKey{}, client))

	cobra.OnFinalize(func() {
		if event, ok := telemetry.EventFor(cmd, args); ok {
			client.Track(event)
		}

		client.Flush(3 * time.Second)

		log.Stop()
	})

	config.SetAPIConsumerTrace(config.CommandPathToTrace(cmd.CommandPath()))

	f.deps.Animation()

	return nil
}

// persistentPostRun flushes telemetry and stops the logger after a command
// completes successfully.
func (f *RootFactory) persistentPostRun(cmd *cobra.Command) error {
	if client, ok := cmd.Context().Value(telemetry.ClientContextKey{}).(*telemetry.Client); ok {
		client.Flush(3 * time.Second)
	}

	log.Stop()

	return nil
}

// registerFlags attaches all persistent flags to the command adder.
func (f *RootFactory) registerFlags(adder *cli.CommandAdder, outputFormat *outputformat.OutputFormat) {
	flags := adder.PersistentFlags()

	flags.String("config", "",
		"path to config file (default location: $HOME/.config/datarobot/drconfig.yaml)")
	flags.BoolP("version", "V", false, "display the version")
	flags.BoolP("verbose", "v", false, "verbose output")
	flags.Bool("debug", false, "debug output")
	flags.Bool("all-commands", false, "display all available commands and their flags in tree format")
	flags.Bool(config.SkipAuthKey, false, "skip authentication checks (for advanced users)")
	flags.Bool("force-interactive", false, "force setup wizards to run even if already completed")
	flags.Duration("plugin-discovery-timeout", 2*time.Second, "timeout for plugin discovery (0s disables)")
	flags.Duration("plugin-update-check-interval", internalPlugin.DefaultUpdateCheckInterval, "cooldown between plugin update checks (0s disables)")
	flags.Bool("skip-plugin-update-check", false, "skip plugin update checks before running plugins")
	flags.Bool("disable-telemetry", false, "disable usage telemetry")

	// Private CA / TLS flags.
	flags.BoolP("skip-certificate-check", "k", false, "skip TLS certificate verification (insecure)")
	flags.String("ca-cert", "", "path to a PEM-encoded CA certificate bundle")
	registerExportWindowsCertsFlag(adder.Command)

	outputformat.AddPersistentFlag(adder.Command, outputFormat)
}

// bindViperFlags wires persistent flags to viper and annotates the universal
// ones for forwarding to plugin subprocesses as DATAROBOT_CLI_* env vars.
func (f *RootFactory) bindViperFlags(adder *cli.CommandAdder) {
	// bindUniversal binds name to viper AND annotates it for plugin forwarding.
	bindUniversalOn := func(name string) {
		flag := adder.PersistentFlags().Lookup(name)
		_ = viperx.BindPFlag(name, flag)

		if flag.Annotations == nil {
			flag.Annotations = map[string][]string{}
		}

		flag.Annotations[config.UniversalAnnotationKey] = []string{
			strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
		}
	}

	// Universal flags: bound to viper AND forwarded to plugin subprocesses as
	// DATAROBOT_CLI_* env vars. To add a new universal flag, call bindUniversalOn
	// next to its registration in registerFlags.
	bindUniversalOn("debug")
	bindUniversalOn("disable-telemetry")
	bindUniversalOn("verbose")
	bindUniversalOn("skip-certificate-check")
	bindUniversalOn("ca-cert")

	// Non-universal flags: bound to viper only (not forwarded to plugins).
	pflags := adder.PersistentFlags()

	_ = viperx.BindPFlag("config", pflags.Lookup("config"))
	_ = viperx.BindPFlag(config.SkipAuthKey, pflags.Lookup(config.SkipAuthKey))
	_ = viperx.BindPFlag("force-interactive", pflags.Lookup("force-interactive"))
	_ = viperx.BindPFlag("plugin-discovery-timeout", pflags.Lookup("plugin-discovery-timeout"))
	_ = viperx.BindPFlag("plugin-update-check-interval", pflags.Lookup("plugin-update-check-interval"))
	_ = viperx.BindPFlag("skip-plugin-update-check", pflags.Lookup("skip-plugin-update-check"))
	_ = viperx.BindPFlag("output-format", pflags.Lookup("output-format"))
}

// addGroups registers the named command groups used by the help template.
func (f *RootFactory) addGroups(adder *cli.CommandAdder) {
	adder.AddGroup(
		&cobra.Group{ID: "core", Title: tui.BaseTextStyle.Render("Core Commands:")},
		&cobra.Group{ID: "self", Title: tui.BaseTextStyle.Render("Self Commands:")},
		&cobra.Group{ID: "advanced", Title: tui.BaseTextStyle.Render("Advanced Commands:")},
	)
}

// addSubcommands registers all first-party sub-commands on the adder.
// Commands whose feature gates are disabled are silently dropped by
// cli.CommandAdder.AddCommand.
func (f *RootFactory) addSubcommands(adder *cli.CommandAdder) {
	adder.AddCommand(
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
}

// configureHelp installs custom usage/help templates and the --all-commands /
// --version help-func overrides on the adder's root command.
func (f *RootFactory) configureHelp(adder *cli.CommandAdder) {
	adder.SetUsageTemplate(CustomUsageTemplate)

	defaultHelpFunc := adder.HelpFunc()

	adder.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		showAllCommands, _ := cmd.Flags().GetBool("all-commands")
		showVersion, _ := cmd.Flags().GetBool("version")

		switch {
		case showAllCommands:
			output := allCommandsOutput(cmd.Root())
			_, _ = fmt.Fprint(cmd.OutOrStdout(), output)

		case showVersion:
			runVersionCommand(cmd)

		default:
			adder.SetHelpTemplate(CustomHelpTemplate)
			defaultHelpFunc(cmd, args)
		}
	})
}

// ---------------------------------------------------------------------------
// defaultConfigInitializer — production implementation of ConfigInitializerFunc
// ---------------------------------------------------------------------------

// defaultConfigInitializer is the production ConfigInitializerFunc. It sets up
// viper's env-var prefix, binds standard overrides, and reads drconfig.yaml from
// disk. Tests can replace this with a no-op to skip disk I/O and isolate env state.
func defaultConfigInitializer(cmd *cobra.Command, configFilePath string) error {
	// Map any environment variables prefixed with DATAROBOT_CLI_ to config keys.
	viperx.SetEnvPrefix("DATAROBOT_CLI")
	viperx.AutomaticEnv()
	viperx.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Map VISUAL and EDITOR to external-editor, with a safe default.
	viperx.SetDefault("external-editor", "vi")

	_ = viperx.BindEnv("external-editor", "VISUAL", "EDITOR")

	// API consumer tracking is enabled by default; operators can opt out with
	// DATAROBOT_API_CONSUMER_TRACKING_ENABLED=false (matches the Python SDK).
	viperx.SetDefault(config.APIConsumerTrackingEnabled, true)
	_ = viperx.BindEnv(config.APIConsumerTrackingEnabled, "DATAROBOT_API_CONSUMER_TRACKING_ENABLED")

	// Resolve the config file path: flag > env var > default.
	resolved := configFilePath
	if resolved == "" {
		if envPath := viperx.GetString("config"); envPath != "" {
			resolved = envPath
		}
	}

	if err := config.ReadConfigFile(resolved); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers delegated to from configureHelp
// ---------------------------------------------------------------------------

// allCommandsOutput renders the full command tree and is the implementation
// delegated to when --all-commands is set. Exists as a named helper so the
// import of allcommands can live in root.go (the production side) while the
// factory stays import-light. Overridden in root.go via allCommandsOutputFn.
var allCommandsOutputFn func(root *cobra.Command) string

// allCommandsOutput calls the registered output function (set by root.go).
func allCommandsOutput(root *cobra.Command) string {
	if allCommandsOutputFn != nil {
		return allCommandsOutputFn(root)
	}

	return ""
}

// runVersionCommandFn is the implementation delegated to when --version is set
// in a help context. Set by root.go so the factory avoids importing cmd/self/version.
var runVersionCommandFn func(cmd *cobra.Command)

// runVersionCommand calls the registered version runner.
func runVersionCommand(cmd *cobra.Command) {
	if runVersionCommandFn != nil {
		runVersionCommandFn(cmd)
	}
}
