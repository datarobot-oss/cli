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
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/state"
)

const (
	// DefaultUpdateCheckInterval is the default cooldown between update checks for a given plugin.
	DefaultUpdateCheckInterval = 1 * time.Hour

	// updateCheckRegistryTimeout is a short timeout for fetching the registry during update checks.
	// We keep this short so plugin startup isn't noticeably delayed when the network is slow.
	updateCheckRegistryTimeout = 5 * time.Second
)

// UpdateCheckResult contains information about an available plugin update.
type UpdateCheckResult struct {
	PluginName       string
	InstalledVersion string
	LatestVersion    *RegistryVersion
	RegistryPlugin   RegistryPlugin
	BaseURL          string
}

// CheckForUpdate checks whether a newer version of the given managed plugin is available.
//
// The check is skipped (returns nil) when:
//   - The configured check interval is 0 (disabled)
//   - The cooldown period for this plugin has not elapsed yet
//   - The network is unreachable or the registry fetch fails
//   - The installed version is already the latest
//
// The cooldown timestamp is always recorded after a successful registry fetch,
// regardless of whether an update is found. Callers may call state.SetLastPluginCheck
// again afterwards (e.g. after a user interaction) to refresh the timestamp.
func CheckForUpdate(pluginName, installedVersion, registryURL string) *UpdateCheckResult {
	if shouldSkipCheck(pluginName) {
		return nil
	}

	log.Debug("Checking for plugin update", "plugin", pluginName, "installed", installedVersion)

	// Use a short timeout so we don't delay plugin startup when offline
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckRegistryTimeout)
	defer cancel()

	registry, baseURL, err := FetchRegistryWithContext(ctx, registryURL)
	if err != nil {
		// Network errors are non-fatal: no internet, DNS failure, timeout, registry down, etc.
		log.Debug("Plugin update check: registry fetch failed (skipping)", "plugin", pluginName, "error", err)

		return nil
	}

	// Record the cooldown now that we have successfully contacted the registry.
	// This prevents a redundant network request on every subsequent invocation
	// when the plugin is already up-to-date.
	state.SetLastPluginCheck(pluginName)

	return compareVersions(pluginName, installedVersion, registry, baseURL)
}

// shouldSkipCheck returns true when the update check should be skipped
// because it is disabled or the cooldown has not elapsed.
func shouldSkipCheck(pluginName string) bool {
	interval := viperx.GetDuration("plugin-update-check-interval")
	if interval <= 0 {
		log.Debug("Plugin update check disabled via config", "plugin", pluginName)

		return true
	}

	lastCheck := state.GetLastPluginCheck(pluginName)
	if !lastCheck.IsZero() && time.Since(lastCheck) < interval {
		log.Debug("Plugin update check skipped (cooldown active)",
			"plugin", pluginName,
			"lastCheck", lastCheck,
			"interval", interval)

		return true
	}

	return false
}

// compareVersions resolves the latest version from the registry and compares it
// with the installed version. Returns nil when no update is available.
func compareVersions(
	pluginName, installedVersion string,
	registry *PluginRegistry,
	baseURL string,
) *UpdateCheckResult {
	pluginEntry, ok := registry.Plugins[pluginName]
	if !ok {
		log.Debug("Plugin not found in registry", "plugin", pluginName)

		return nil
	}

	latestVersion, err := ResolveVersion(pluginEntry.Versions, "latest")
	if err != nil {
		log.Debug("Failed to resolve latest version", "plugin", pluginName, "error", err)

		return nil
	}

	if !isNewerVersion(installedVersion, latestVersion.Version) {
		return nil
	}

	return &UpdateCheckResult{
		PluginName:       pluginName,
		InstalledVersion: installedVersion,
		LatestVersion:    latestVersion,
		RegistryPlugin:   pluginEntry,
		BaseURL:          baseURL,
	}
}

// isNewerVersion returns true if latestStr is strictly greater than installedStr.
func isNewerVersion(installedStr, latestStr string) bool {
	installed, err := semver.NewVersion(installedStr)
	if err != nil {
		// If installed version isn't valid semver, fall back to string comparison
		return installedStr != latestStr
	}

	latest, err := semver.NewVersion(latestStr)
	if err != nil {
		return false
	}

	return latest.GreaterThan(installed)
}
