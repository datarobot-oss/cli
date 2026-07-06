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
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/config"
)

// ManagedPluginsDir returns the user-global managed plugins directory.
// It respects XDG_CONFIG_HOME if set, otherwise falls back to ~/.config/datarobot/plugins/
func ManagedPluginsDir() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "plugins"), nil
}

// ManagedPluginsDirs returns all managed plugin directories to search, in priority order.
// When XDG_CONFIG_HOME is set, the XDG path is checked first, then ~/.config/datarobot/plugins
// as a fallback so plugins installed without XDG_CONFIG_HOME remain discoverable.
func ManagedPluginsDirs() ([]string, error) {
	primaryDir, err := ManagedPluginsDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{primaryDir}

	if os.Getenv("XDG_CONFIG_HOME") != "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			defaultDir := filepath.Join(homeDir, ".config", "datarobot", "plugins")

			if defaultDir != primaryDir {
				dirs = append(dirs, defaultDir)
			}
		}
	}

	return dirs, nil
}
