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

package state

import (
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/config"
	"gopkg.in/yaml.v3"
)

const globalStateFileName = "state.yaml"

// globalState represents CLI state that is user-global (not project-scoped).
// Stored in ~/.config/datarobot/state.yaml (or $XDG_CONFIG_HOME/datarobot/state.yaml).
type globalState struct {
	fullPath string
	// PluginUpdateChecks maps plugin name → last check timestamp (UTC).
	PluginUpdateChecks map[string]time.Time `yaml:"plugin_update_checks,omitempty"`
}

// globalStatePath returns the path to the global state file.
func globalStatePath() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, globalStateFileName), nil
}

// loadGlobalState reads the global state file.
// Returns a zero-value state (not an error) if the file doesn't exist.
func loadGlobalState() (globalState, error) {
	fullPath, err := globalStatePath()
	if err != nil {
		return globalState{}, err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return globalState{fullPath: fullPath}, nil
		}

		return globalState{}, err
	}

	var gs globalState

	if err := yaml.Unmarshal(data, &gs); err != nil {
		// If the file is corrupted, start fresh rather than failing
		return globalState{fullPath: fullPath}, nil
	}

	gs.fullPath = fullPath

	return gs, nil
}

// save writes the global state file to disk.
// Creates parent directories if they don't exist.
func (gs globalState) save() error {
	stateDir := filepath.Dir(gs.fullPath)

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(gs)
	if err != nil {
		return err
	}

	return os.WriteFile(gs.fullPath, data, 0o644)
}

// GetLastPluginCheck returns the last time an update check was performed for the given plugin.
// Returns the zero time if the plugin has never been checked.
func GetLastPluginCheck(pluginName string) time.Time {
	gs, err := loadGlobalState()
	if err != nil {
		return time.Time{}
	}

	if gs.PluginUpdateChecks == nil {
		return time.Time{}
	}

	return gs.PluginUpdateChecks[pluginName]
}

// SetLastPluginCheck records the current time as the last update-check time for a plugin.
// Errors are silently ignored — failing to persist state should never block plugin execution.
func SetLastPluginCheck(pluginName string) {
	gs, err := loadGlobalState()
	if err != nil {
		return
	}

	if gs.PluginUpdateChecks == nil {
		gs.PluginUpdateChecks = make(map[string]time.Time)
	}

	gs.PluginUpdateChecks[pluginName] = time.Now().UTC()

	_ = gs.save()
}
