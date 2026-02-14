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

package update

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"gopkg.in/yaml.v3"
)

func createTestConfig(t *testing.T, path, projectName string, enableAuth bool, pythonVersions []interface{}) {
	t.Helper()

	cfg := config.ComponentDefaults{
		Defaults: map[string]map[string]interface{}{
			"https://github.com/example/template.git": {
				"project_name":    projectName,
				"enable_auth":     enableAuth,
				"python_versions": pythonVersions,
			},
		},
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
}

func TestConfigHierarchy(t *testing.T) {
	// Create temporary directories for testing
	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "repo")
	homeDir := filepath.Join(tempDir, "home")

	if err := os.MkdirAll(filepath.Join(repoRoot, ".datarobot"), 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "datarobot"), 0o755); err != nil {
		t.Fatalf("Failed to create home dir: %v", err)
	}

	// Create test configs
	repoConfigPath := filepath.Join(repoRoot, ".datarobot", config.ComponentDefaultsFileName)
	createTestConfig(t, repoConfigPath, "RepoProject", true, []interface{}{3.10, 3.11})

	homeConfigPath := filepath.Join(homeDir, ".config", "datarobot", config.ComponentDefaultsFileName)
	createTestConfig(t, homeConfigPath, "HomeProject", false, []interface{}{3.9})

	explicitConfigPath := filepath.Join(tempDir, "explicit.yaml")
	createTestConfig(t, explicitConfigPath, "ExplicitProject", true, []interface{}{3.12})

	tests := []struct {
		name         string
		explicitPath string
		expectedName string
		expectedAuth interface{}
	}{
		{
			name:         "explicit path takes priority over all",
			explicitPath: explicitConfigPath,
			expectedName: "ExplicitProject",
			expectedAuth: true,
		},
		{
			name:         "repo config via explicit path",
			explicitPath: repoConfigPath,
			expectedName: "RepoProject",
			expectedAuth: true,
		},
		{
			name:         "home config via explicit path",
			explicitPath: homeConfigPath,
			expectedName: "HomeProject",
			expectedAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.LoadComponentDefaults(tt.explicitPath)
			if err != nil {
				t.Fatalf("LoadComponentDefaults() error = %v", err)
			}

			defaults := cfg.GetDefaultsForRepo("https://github.com/example/template.git")
			if defaults["project_name"] != tt.expectedName {
				t.Errorf("Expected project_name = %s, got %v", tt.expectedName, defaults["project_name"])
			}

			if defaults["enable_auth"] != tt.expectedAuth {
				t.Errorf("Expected enable_auth = %v, got %v", tt.expectedAuth, defaults["enable_auth"])
			}
		})
	}
}

func TestMergeWithCLIData(t *testing.T) {
	config := &config.ComponentDefaults{
		Defaults: map[string]map[string]interface{}{
			"https://github.com/example/template.git": {
				"project_name":    "ConfigProject",
				"enable_auth":     true,
				"python_versions": []interface{}{3.10, 3.11},
				"port":            8080,
			},
		},
	}

	tests := []struct {
		name     string
		cliData  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:    "no CLI data - use defaults",
			cliData: map[string]interface{}{},
			expected: map[string]interface{}{
				"project_name":    "ConfigProject",
				"enable_auth":     true,
				"python_versions": []interface{}{3.10, 3.11},
				"port":            8080,
			},
		},
		{
			name: "CLI data overrides defaults",
			cliData: map[string]interface{}{
				"project_name": "CLIProject",
				"enable_auth":  false,
			},
			expected: map[string]interface{}{
				"project_name":    "CLIProject",
				"enable_auth":     false,
				"python_versions": []interface{}{3.10, 3.11},
				"port":            8080,
			},
		},
		{
			name: "CLI adds new keys",
			cliData: map[string]interface{}{
				"new_feature": true,
				"timeout":     30.5,
			},
			expected: map[string]interface{}{
				"project_name":    "ConfigProject",
				"enable_auth":     true,
				"python_versions": []interface{}{3.10, 3.11},
				"port":            8080,
				"new_feature":     true,
				"timeout":         30.5,
			},
		},
		{
			name: "CLI overrides list values",
			cliData: map[string]interface{}{
				"python_versions": []interface{}{3.12},
			},
			expected: map[string]interface{}{
				"project_name":    "ConfigProject",
				"enable_auth":     true,
				"python_versions": []interface{}{3.12},
				"port":            8080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.MergeWithCLIData("https://github.com/example/template.git", tt.cliData)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				assertValueEquals(t, key, expectedValue, result[key])
			}
		})
	}
}

func assertValueEquals(t *testing.T, key string, expected, actual interface{}) {
	t.Helper()

	if actual == nil {
		t.Errorf("Missing key %s", key)

		return
	}

	// For list comparison, compare lengths and elements
	if expectedList, ok := expected.([]interface{}); ok {
		actualList, ok := actual.([]interface{})
		if !ok {
			t.Errorf("Key %s: expected list, got %T", key, actual)

			return
		}

		if len(expectedList) != len(actualList) {
			t.Errorf("Key %s: expected list length %d, got %d", key, len(expectedList), len(actualList))

			return
		}

		for i := range expectedList {
			if expectedList[i] != actualList[i] {
				t.Errorf("Key %s[%d]: expected %v, got %v", key, i, expectedList[i], actualList[i])
			}
		}
	} else if actual != expected {
		t.Errorf("Key %s: expected %v, got %v", key, expected, actual)
	}
}
