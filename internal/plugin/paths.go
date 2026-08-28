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
	"path/filepath"

	"github.com/datarobot/cli/internal/config"
)

// ManagedPluginsDir returns the user-global managed plugins directory.
// It respects XDG_CONFIG_HOME if set, otherwise falls back to ~/.config/datarobot/plugins/.
func ManagedPluginsDir() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "plugins"), nil
}

// ManagedPluginsDirs returns all managed plugin directories to search, in priority order.
// Respects XDG_CONFIG_DIRS for additional search paths (only when explicitly set by the user).
// Priority order:
//  1. Primary directory from XDG_CONFIG_HOME (or ~/.config/datarobot/plugins)
//  2. Directories from XDG_CONFIG_DIRS environment variable (if set)
func ManagedPluginsDirs() ([]string, error) {
	primaryDir, err := ManagedPluginsDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{primaryDir}
	seen := map[string]bool{primaryDir: true}

	// Add directories from XDG_CONFIG_DIRS (only if explicitly set)
	for _, dir := range config.GetConfigDirs() {
		pluginDir := filepath.Join(dir, "datarobot", "plugins")

		// Deduplicate
		if !seen[pluginDir] {
			dirs = append(dirs, pluginDir)
			seen[pluginDir] = true
		}
	}

	return dirs, nil
}
