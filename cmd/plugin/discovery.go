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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/plugin/shared"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// RegisterPluginCommands discovers installed plugins and registers them as sub-commands
// on rootCmd. The plugin group is only added when at least one plugin is found.
func RegisterPluginCommands(rootCmd *cobra.Command) {
	timeout := viperx.GetDuration("plugin-discovery-timeout")
	if timeout <= 0 {
		log.Debug("Plugin discovery disabled", "timeout", timeout)

		return
	}

	// Get list of builtin command names FIRST (before adding plugins)
	builtinNames := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
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

func createPluginCommand(p internalPlugin.DiscoveredPlugin) *cobra.Command {
	executable := p.Executable // Capture for closure
	manifest := p.Manifest     // Capture for closure
	pluginName := p.Manifest.Name
	pluginPath := p.Executable // Used to determine if managed

	return &cobra.Command{
		Use:                p.Manifest.Name,
		Short:              p.Manifest.Description,
		GroupID:            "plugin",
		DisableFlagParsing: true, // Pass all args to plugin
		DisableSuggestions: true,
		Run: func(_ *cobra.Command, args []string) {
			checkAndPromptPluginUpdate(pluginName, manifest.Version, pluginPath)

			fmt.Println(tui.InfoStyle.Render("🔌 Running plugin: " + pluginName))
			log.Debug("Executing plugin", "name", pluginName, "executable", executable)

			exitCode := internalPlugin.ExecutePlugin(manifest, executable, args)
			os.Exit(exitCode)
		},
	}
}

// checkAndPromptPluginUpdate checks if an update is available for a managed plugin.
// If one is found it prompts the user to upgrade.
// Non-managed plugins (PATH-based, project-local) are silently skipped.
// Cooldown tracking is handled entirely inside CheckForUpdate: the timestamp is
// recorded only after a successful registry fetch, so skipped (cooldown-active)
// invocations never push the timestamp forward.
func checkAndPromptPluginUpdate(pluginName, installedVersion, pluginPath string) {
	if viperx.GetBool("skip-plugin-update-check") {
		return
	}

	// Only check managed plugins (those under ~/.config/datarobot/plugins/)
	if !isManagedPlugin(pluginPath) {
		log.Debug("Plugin update check skipped (not a managed plugin)", "plugin", pluginName, "path", pluginPath)

		return
	}

	result := internalPlugin.CheckForUpdate(pluginName, installedVersion, internalPlugin.PluginRegistryURL)
	if result == nil {
		return
	}

	// Don't prompt when stdin is not a terminal (piped input, CI, scripts).
	// Consuming stdin here would corrupt the data the plugin is about to read.
	if !reader.IsStdinTerminal() {
		log.Debug("Plugin update available but stdin is not a terminal — skipping prompt",
			"plugin", pluginName,
			"available", result.LatestVersion.Version)

		return
	}

	// An update is available — prompt the user
	fmt.Println(tui.InfoStyle.Render(
		fmt.Sprintf("Plugin %q update available: v%s → v%s",
			result.PluginName, result.InstalledVersion, result.LatestVersion.Version)))
	fmt.Print(tui.DimStyle.Render("Do you want to update? [Y/n] "))

	if !askYesNo() {
		log.Debug("Plugin update declined by user", "plugin", pluginName)
		fmt.Println()

		return
	}

	performPluginUpdate(result)

	fmt.Println()
}

// isManagedPlugin returns true if the plugin executable lives under the managed plugins directory.
func isManagedPlugin(pluginPath string) bool {
	managedDir, err := internalPlugin.ManagedPluginsDir()
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(managedDir, pluginPath)
	if err != nil {
		return false
	}

	// If the relative path starts with ".." the plugin is outside the managed dir
	return !strings.HasPrefix(rel, "..")
}

// askYesNo reads a single line from stdin and returns true unless the user explicitly declines.
// Default is yes (empty input / just pressing Enter returns true).
func askYesNo() bool {
	return reader.AskYesNo()
}

// performPluginUpdate runs the backup → install → validate cycle for a plugin update.
func performPluginUpdate(result *internalPlugin.UpdateCheckResult) {
	shared.RunPluginUpdate(
		result.PluginName,
		result.InstalledVersion,
		result.RegistryPlugin,
		*result.LatestVersion,
		result.BaseURL,
	)
}
