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

package plugin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/plugin/shared"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/outputformat"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// RegisterPluginCommands discovers installed plugins and registers them as sub-commands
// on rootCmd. The plugin group is only added when at least one plugin is found.
func RegisterPluginCommands(rootCmd *cobra.Command) {
	timeout := pluginDiscoveryTimeout(rootCmd)
	if timeout <= 0 {
		log.Debug("Plugin discovery disabled", "timeout", timeout)

		// Seed the cache as empty so a later lazy GetPlugins() call (e.g. from
		// `dr plugin list`) cannot re-run discovery under a lower-priority
		// timeout source, such as an env value undoing an explicit 0s flag.
		internalPlugin.PrimeCache(nil, nil)

		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Get list of builtin command names FIRST (before adding plugins)
	builtinNames := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		builtinNames[cmd.Name()] = true
	}

	plugins, conflicts := internalPlugin.DiscoverPluginsWithContext(ctx)

	// Registering commands considers every discovered plugin, so every
	// conflict is relevant here (it affects which binary wins a command name).
	internalPlugin.LogConflicts(conflicts)

	// Seed the shared discovery cache so a later plugin.GetPlugins() call
	// (e.g. from `dr plugin list` or `dr plugin version`) reuses this result
	// instead of re-scanning PATH from scratch.
	internalPlugin.PrimeCache(plugins, conflicts)

	if len(plugins) == 0 {
		// No plugins found, don't add empty group header
		return
	}

	// Only add plugin group if we have plugins to show
	rootCmd.AddGroup(&cobra.Group{
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

		rootCmd.AddCommand(createPluginCommand(p))
	}
}

// pluginDiscoveryTimeout resolves the startup discovery timeout. Discovery
// happens before config initialization, so the root flag is read directly when
// it was pre-parsed, and the env var is read directly instead of through viper.
func pluginDiscoveryTimeout(rootCmd *cobra.Command) time.Duration {
	if flag := rootCmd.PersistentFlags().Lookup(internalPlugin.DiscoveryTimeoutKey); flag != nil && flag.Changed {
		timeout, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			log.Debug("Invalid plugin discovery timeout flag", "value", flag.Value.String(), "error", err)

			return internalPlugin.DefaultDiscoveryTimeout
		}

		return timeout
	}

	return internalPlugin.DiscoveryTimeout()
}

func createPluginCommand(p internalPlugin.DiscoveredPlugin) *cobra.Command {
	executable := p.Executable // Capture for closure
	manifest := p.Manifest     // Capture for closure
	pluginName := p.Manifest.Name
	pluginPath := p.Executable // Used to determine if managed

	cmd := &cobra.Command{
		Use:                p.Manifest.Name,
		Short:              p.Manifest.Description,
		GroupID:            "plugin",
		DisableFlagParsing: true, // Pass all args to plugin
		DisableSuggestions: true,
		Run: func(pluginCmd *cobra.Command, args []string) {
			shared.CheckAndPromptPluginUpdate(pluginName, manifest.Version, pluginPath)

			if !suppressBanner(pluginCmd, args) {
				// The banner goes to stderr so stdout stays reserved for the
				// plugin's own output (e.g. under --output-format json). It is
				// styled for stderr, not with the package style, which probes
				// stdout: otherwise `2> file` from a terminal would capture
				// escape codes.
				fmt.Fprintln(os.Stderr, tui.StylesFor(os.Stderr).Info.Render("🔌 Running plugin: "+pluginName))
			}

			log.Debug("Executing plugin", "name", pluginName, "executable", executable)

			exitCode := internalPlugin.ExecutePlugin(pluginCmd.Context(), manifest, executable, args, pluginCmd.Root().PersistentFlags())

			telemetry.ExitCommand(pluginCmd, args, exitCode)
		},
	}

	telemetry.TrackPlugin(cmd, manifest.Version)

	return cmd
}

// suppressBanner reports whether the "Running plugin" banner should be skipped.
// The banner is a human-facing courtesy, so it is dropped when the output is
// meant for a machine: when JSON output is requested (so that
// `--output-format json 2>&1 | jq .` parses) and when the run is
// non-interactive (DATAROBOT_CLI_NON_INTERACTIVE).
//
// Plugin commands set DisableFlagParsing, so cobra never parses the flags that
// follow the plugin name. A `--yes` or `-y` there is handed to the plugin and
// is not seen by cli.IsNonInteractive; only the environment variable is. The
// same limitation applies to the `--output-format` flag, which is why
// jsonOutputRequested scans args itself.
func suppressBanner(cmd *cobra.Command, args []string) bool {
	return cli.IsNonInteractive(cmd) || jsonOutputRequested(cmd, args)
}

// jsonOutputRequested reports whether the invocation asks for JSON output. An
// explicit `--output-format` in args wins, as it does for every other command;
// otherwise the format falls back to the env var / config file value that
// outputformat.GetFormat resolves (DATAROBOT_CLI_OUTPUT_FORMAT).
//
// Only the `--output-format` flag is recognised. The deprecated `--format` and
// `-o` aliases exist on `dr self version` alone, and on a plugin command line
// `-o` belongs to the plugin and may mean something else entirely.
func jsonOutputRequested(cmd *cobra.Command, args []string) bool {
	if format, found := outputFormatFromArgs(args); found {
		return format == string(outputformat.OutputFormatJSON)
	}

	return outputformat.GetFormat(cmd) == outputformat.OutputFormatJSON
}

// outputFormatFromArgs returns the value of the last `--output-format` in args,
// in either the `--output-format json` or `--output-format=json` form. found is
// false when the flag is absent. Scanning stops at a bare `--`, after which
// everything is a positional argument rather than a flag.
func outputFormatFromArgs(args []string) (format string, found bool) {
	const flag = "--output-format"

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			return format, found
		case arg == flag && i+1 < len(args):
			format, found = args[i+1], true
			i++
		case strings.HasPrefix(arg, flag+"="):
			format, found = strings.TrimPrefix(arg, flag+"="), true
		}
	}

	return format, found
}

// The update-check flow (prompt, backup → install → validate) lives in
// cmd/plugin/shared so wrapper commands that launch plugins directly
// (e.g. dr build-studio) share it. See shared.CheckAndPromptPluginUpdate.
