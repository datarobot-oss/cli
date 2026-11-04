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

package shared

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
)

// CheckAndPromptPluginUpdate checks if an update is available for a managed
// plugin and, if one is found, prompts the user to upgrade. Non-managed
// plugins (PATH-based, project-local) are silently skipped, as is any call
// when stdin is not a terminal or update checks are disabled. Cooldown
// tracking is handled entirely inside CheckForUpdate: the timestamp is
// recorded only after a successful registry fetch, so skipped
// (cooldown-active) invocations never push the timestamp forward.
func CheckAndPromptPluginUpdate(pluginName, installedVersion, pluginPath string) {
	if viperx.GetBool("skip-plugin-update-check") {
		return
	}

	// Only check managed plugins (those under ~/.config/datarobot/plugins/).
	if !isManagedPlugin(pluginPath) {
		log.Debug("Plugin update check skipped (not a managed plugin)", "plugin", pluginName, "path", pluginPath)

		return
	}

	result := plugin.CheckForUpdate(pluginName, installedVersion, plugin.PluginRegistryURL)
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

	// An update is available — prompt the user.
	fmt.Println(tui.InfoStyle.Render(
		fmt.Sprintf("Plugin %q update available: v%s → v%s",
			result.PluginName, result.InstalledVersion, result.LatestVersion.Version),
	))
	fmt.Print(tui.DimStyle.Render("Do you want to update? [Y/n] "))

	if !reader.AskYesNo() {
		log.Debug("Plugin update declined by user", "plugin", pluginName)
		fmt.Println()

		return
	}

	performPluginUpdate(result)

	fmt.Println()
}

// isManagedPlugin returns true if the plugin executable lives under any managed plugins directory.
// Checks the primary directory (XDG_CONFIG_HOME) and any directories listed in XDG_CONFIG_DIRS.
func isManagedPlugin(pluginPath string) bool {
	managedDirs, err := plugin.ManagedPluginsDirs()
	if err != nil {
		return false
	}

	for _, managedDir := range managedDirs {
		rel, err := filepath.Rel(managedDir, pluginPath)
		if err != nil {
			continue
		}

		if !strings.HasPrefix(rel, "..") {
			return true
		}
	}

	return false
}

// performPluginUpdate runs the backup → install → validate cycle for a plugin update.
func performPluginUpdate(result *plugin.UpdateCheckResult) {
	err := RunPluginUpdate(
		result.PluginName,
		result.InstalledVersion,
		result.RegistryPlugin,
		*result.LatestVersion,
		result.BaseURL,
	)
	if err != nil {
		fmt.Println(tui.ErrorStyle.Render(fmt.Sprintf("✗ Failed to update %s: %v", result.PluginName, err)))

		return
	}

	fmt.Println(tui.SuccessStyle.Render("✓ Updated " + result.PluginName + " to " + result.LatestVersion.Version))
}
