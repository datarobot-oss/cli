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
	"time"

	"github.com/datarobot/cli/cmd/plugin/shared"
	"github.com/datarobot/cli/internal/log"
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

			// The banner goes to stderr so stdout stays reserved for the
			// plugin's own output (e.g. under --output-format json).
			fmt.Fprintln(os.Stderr, tui.InfoStyle.Render("🔌 Running plugin: "+pluginName))
			log.Debug("Executing plugin", "name", pluginName, "executable", executable)

			exitCode := internalPlugin.ExecutePlugin(pluginCmd.Context(), manifest, executable, args, pluginCmd.Root().PersistentFlags())

			// Enqueue the command's telemetry event before exiting: os.Exit
			// skips the OnFinalize hook that normally fires it.
			telemetry.TrackCommandFromContext(pluginCmd.Context(), pluginCmd, args)
			telemetry.ExitWithContext(pluginCmd.Context(), exitCode)
		},
	}

	telemetry.TrackPlugin(cmd, manifest.Version)

	return cmd
}

// The update-check flow (prompt, backup → install → validate) lives in
// cmd/plugin/shared so wrapper commands that launch plugins directly
// (e.g. dr build-studio) share it. See shared.CheckAndPromptPluginUpdate.
