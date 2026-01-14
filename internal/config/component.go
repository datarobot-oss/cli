// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/repo"
	"gopkg.in/yaml.v3"
)

const (
	// ComponentDefaultsFileName is the name of the component defaults file
	// This follows copier's data_file convention
	ComponentDefaultsFileName = ".copier-answers-defaults.yaml"
	// LegacyComponentDefaultsFileName is the old name for backward compatibility
	LegacyComponentDefaultsFileName = "component-defaults.yaml"
)

// ComponentDefaults holds default answers for copier templates
// The structure maps component repo URLs to their default answers
// This follows copier's data_file semantics but supports multiple repos
type ComponentDefaults struct {
	Defaults map[string]map[string]interface{} `yaml:"defaults"`
}

// LoadComponentDefaults reads the component defaults configuration file
// Priority order:
// 1. Explicitly provided path (if not empty)
// 2. .datarobot/.copier-answers-defaults.yaml in repo root
// 3. ~/.config/datarobot/.copier-answers-defaults.yaml
// 4. ~/.config/datarobot/component-defaults.yaml (legacy)
// Returns an empty config if no file exists
func LoadComponentDefaults(explicitPath string) (*ComponentDefaults, error) {
	var configPath string

	if explicitPath != "" {
		configPath = explicitPath
	} else {
		configPath = findComponentDefaultsPath()
	}

	if configPath == "" {
		// No config file found, return empty config
		return &ComponentDefaults{
			Defaults: make(map[string]map[string]interface{}),
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ComponentDefaults{
				Defaults: make(map[string]map[string]interface{}),
			}, nil
		}

		return nil, fmt.Errorf("failed to read component defaults file: %w", err)
	}

	var config ComponentDefaults

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal component defaults: %w", err)
	}

	if config.Defaults == nil {
		config.Defaults = make(map[string]map[string]interface{})
	}

	return &config, nil
}

// findComponentDefaultsPath searches for the component defaults file
// in priority order and returns the first one found
func findComponentDefaultsPath() string {
	// 1. Check repo root .datarobot folder
	repoRoot, err := repo.FindRepoRoot()
	if err == nil {
		repoPath := filepath.Join(repoRoot, ".datarobot", ComponentDefaultsFileName)
		if _, err := os.Stat(repoPath); err == nil {
			return repoPath
		}
	}

	// 2. Check home directory .config/datarobot
	homeConfigDir := filepath.Join(os.Getenv("HOME"), configFileDir)

	homePath := filepath.Join(homeConfigDir, ComponentDefaultsFileName)
	if _, err := os.Stat(homePath); err == nil {
		return homePath
	}

	// 3. Check legacy filename for backward compatibility
	legacyPath := filepath.Join(homeConfigDir, LegacyComponentDefaultsFileName)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}

	return ""
}

// GetDefaultsForRepo returns the default answers for a specific repository URL
// Returns an empty map if no defaults are configured
func (c *ComponentDefaults) GetDefaultsForRepo(repoURL string) map[string]interface{} {
	if c.Defaults == nil {
		return make(map[string]interface{})
	}

	defaults, ok := c.Defaults[repoURL]
	if !ok {
		return make(map[string]interface{})
	}

	return defaults
}

// MergeWithCLIData merges configured defaults with CLI-provided --data arguments
// CLI arguments take precedence over defaults
func (c *ComponentDefaults) MergeWithCLIData(repoURL string, cliData map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Start with defaults
	for k, v := range c.GetDefaultsForRepo(repoURL) {
		result[k] = v
	}

	// Override with CLI data
	for k, v := range cliData {
		result[k] = v
	}

	return result
}
