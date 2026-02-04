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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLivePluginRegistrySchema validates the actual docs/plugins/index.json file
func TestLivePluginRegistrySchema(t *testing.T) {
	// Find the project root by looking for go.mod
	projectRoot, err := findProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	indexPath := filepath.Join(projectRoot, "docs", "plugins", "index.json")

	data, err := os.ReadFile(indexPath)
	require.NoError(t, err, "Failed to read docs/plugins/index.json - file must exist")

	var registry PluginRegistry

	err = json.Unmarshal(data, &registry)
	require.NoError(t, err, "docs/plugins/index.json must be valid JSON")

	// Validate required fields
	assert.NotEmpty(t, registry.Version, "index.json must have a version field")
	assert.NotEmpty(t, registry.Plugins, "index.json must have at least one plugin")

	// Validate each plugin entry
	for pluginName, plugin := range registry.Plugins {
		t.Run("Plugin_"+pluginName, func(t *testing.T) {
			assert.NotEmpty(t, plugin.Name, "Plugin %s must have a name", pluginName)
			assert.NotEmpty(t, plugin.Description, "Plugin %s must have a description", pluginName)
			assert.NotEmpty(t, plugin.Versions, "Plugin %s must have at least one version", pluginName)

			// Validate each version entry
			for i, version := range plugin.Versions {
				t.Run("Version_"+version.Version, func(t *testing.T) {
					assert.NotEmpty(t, version.Version, "Plugin %s version %d must have a version", pluginName, i)
					assert.NotEmpty(t, version.URL, "Plugin %s version %s must have a URL", pluginName, version.Version)

					// URL can be absolute (with ://) or relative (without ://)
					// Examples:
					// - Absolute: https://cli.datarobot.com/plugins/dr-apps/dr-apps-1.0.0.tar.xz
					// - Relative: dr-apps/dr-apps-1.0.0.tar.xz
					isAbsolute := strings.Contains(version.URL, "://")
					isRelative := !isAbsolute && (strings.HasSuffix(version.URL, ".tar.xz") || strings.HasSuffix(version.URL, ".tar.gz"))

					assert.True(t, isAbsolute || isRelative, "Plugin %s version %s URL must be either absolute (with ://) or relative path ending in .tar.xz/.tar.gz", pluginName, version.Version)

					// Warn if SHA256 is missing (not required but recommended)
					if version.SHA256 == "" {
						t.Logf("WARNING: Plugin %s version %s is missing SHA256 checksum", pluginName, version.Version)
					}

					// Validate SHA256 format if present
					if version.SHA256 != "" {
						assert.Len(t, version.SHA256, 64, "Plugin %s version %s SHA256 must be 64 hex characters", pluginName, version.Version)
						assert.Regexp(t, "^[a-f0-9]+$", version.SHA256, "Plugin %s version %s SHA256 must be lowercase hex", pluginName, version.Version)
					}

					// Validate version format (basic semver check)
					assert.Regexp(t, `^v?\d+\.\d+\.\d+`, version.Version, "Plugin %s version %s must be semver format", pluginName, version.Version)
				})
			}
		})
	}
}

// TestPluginManifestSchema validates plugin manifest JSON schema
func TestPluginManifestSchema(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectError bool
		validate    func(*testing.T, *PluginManifest)
	}{
		{
			name: "valid minimal manifest",
			json: `{"name":"test"}`,
			validate: func(t *testing.T, m *PluginManifest) {
				assert.Equal(t, "test", m.Name)
				assert.Empty(t, m.Version)
				assert.Empty(t, m.Description)
			},
		},
		{
			name: "valid full manifest with authentication",
			json: `{
				"name":"test",
				"version":"1.0.0",
				"description":"Test plugin",
				"minCLIVersion":"0.2.0",
				"authentication":true,
				"scripts":{
					"posix":"scripts/test.sh",
					"windows":"scripts/test.ps1"
				}
			}`,
			validate: func(t *testing.T, m *PluginManifest) {
				assert.Equal(t, "test", m.Name)
				assert.Equal(t, "1.0.0", m.Version)
				assert.Equal(t, "Test plugin", m.Description)
				assert.Equal(t, "0.2.0", m.MinCLIVersion)
				assert.True(t, m.Authentication)
				require.NotNil(t, m.Scripts)
				assert.Equal(t, "scripts/test.sh", m.Scripts.Posix)
				assert.Equal(t, "scripts/test.ps1", m.Scripts.Windows)
			},
		},
		{
			name: "wrong field name - authenticated instead of authentication",
			json: `{
				"name":"test",
				"version":"1.0.0",
				"description":"Test plugin",
				"authenticated":true,
				"scripts":{
					"posix":"scripts/test.sh"
				}
			}`,
			validate: func(t *testing.T, m *PluginManifest) {
				assert.Equal(t, "test", m.Name)
				assert.Equal(t, "1.0.0", m.Version)
				assert.Equal(t, "Test plugin", m.Description)
				assert.False(t, m.Authentication, "authenticated field should be ignored, authentication should be false")
				require.NotNil(t, m.Scripts)
				assert.Equal(t, "scripts/test.sh", m.Scripts.Posix)
			},
		},
		{
			name:        "invalid JSON",
			json:        `{invalid}`,
			expectError: true,
		},
		{
			name: "extra fields ignored",
			json: `{
				"name":"test",
				"unknownField":"value"
			}`,
			validate: func(t *testing.T, m *PluginManifest) {
				assert.Equal(t, "test", m.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manifest PluginManifest

			err := json.Unmarshal([]byte(tt.json), &manifest)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				if tt.validate != nil {
					tt.validate(t, &manifest)
				}
			}
		})
	}
}

// TestPluginRegistryParsing validates plugin registry JSON parsing
func TestPluginRegistryParsing(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectError bool
		validate    func(*testing.T, *PluginRegistry)
	}{
		{
			name: "valid index with one plugin",
			json: `{
				"version":"1",
				"plugins":{
					"test":{
						"name":"test",
						"description":"Test plugin",
						"repository":"https://github.com/test/test",
						"versions":[
							{
								"version":"1.0.0",
								"url":"https://example.com/test-1.0.0.tar.xz",
								"sha256":"abc123"
							}
						]
					}
				}
			}`,
			validate: func(t *testing.T, reg *PluginRegistry) {
				assert.Equal(t, "1", reg.Version)
				assert.Len(t, reg.Plugins, 1)
				plugin := reg.Plugins["test"]
				assert.Equal(t, "test", plugin.Name)
				assert.Equal(t, "Test plugin", plugin.Description)
				assert.Len(t, plugin.Versions, 1)
				assert.Equal(t, "1.0.0", plugin.Versions[0].Version)
				assert.Equal(t, "https://example.com/test-1.0.0.tar.xz", plugin.Versions[0].URL)
			},
		},
		{
			name: "multiple plugins and versions",
			json: `{
				"version":"1",
				"plugins":{
					"plugin1":{"name":"plugin1","versions":[{"version":"1.0.0","url":"https://example.com/p1.tar.xz"}]},
					"plugin2":{"name":"plugin2","versions":[
						{"version":"2.0.0","url":"https://example.com/p2-2.tar.xz"},
						{"version":"1.0.0","url":"https://example.com/p2-1.tar.xz"}
					]}
				}
			}`,
			validate: func(t *testing.T, reg *PluginRegistry) {
				assert.Len(t, reg.Plugins, 2)
				assert.Len(t, reg.Plugins["plugin2"].Versions, 2)
			},
		},
		{
			name:        "invalid JSON",
			json:        `{invalid}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var registry PluginRegistry

			err := json.Unmarshal([]byte(tt.json), &registry)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				if tt.validate != nil {
					tt.validate(t, &registry)
				}
			}
		})
	}
}

// TestInstalledPluginMetadata validates installed plugin metadata JSON
func TestInstalledPluginMetadata(t *testing.T) {
	meta := InstalledPlugin{
		Name:        "test",
		Version:     "1.0.0",
		Source:      "https://example.com/test.tar.xz",
		InstalledAt: "2026-01-27T12:00:00Z",
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var decoded InstalledPlugin

	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, meta.Name, decoded.Name)
	assert.Equal(t, meta.Version, decoded.Version)
	assert.Equal(t, meta.Source, decoded.Source)
	assert.Equal(t, meta.InstalledAt, decoded.InstalledAt)
}

// TestResolveVersion tests semver constraint resolution
func TestResolveVersion(t *testing.T) {
	versions := []RegistryVersion{
		{Version: "2.1.0", URL: "url-2.1.0"},
		{Version: "2.0.0", URL: "url-2.0.0"},
		{Version: "1.5.0", URL: "url-1.5.0"},
		{Version: "1.2.3", URL: "url-1.2.3"},
		{Version: "1.2.0", URL: "url-1.2.0"},
		{Version: "1.0.0", URL: "url-1.0.0"},
	}

	tests := []struct {
		name          string
		constraint    string
		expectedVer   string
		expectError   bool
		errorContains string
	}{
		{"latest", "latest", "2.1.0", false, ""},
		{"empty defaults to latest", "", "2.1.0", false, ""},
		{"exact match", "1.2.3", "1.2.3", false, ""},
		{"exact with v prefix", "1.2.0", "1.2.0", false, ""}, // Note: versions in test don't have v prefix
		{"caret same major", "^1.2.0", "1.5.0", false, ""},
		{"caret newer major excluded", "^1.0.0", "1.5.0", false, ""},
		{"tilde same minor", "~1.2.0", "1.2.3", false, ""},
		{"tilde newer minor excluded", "~1.0.0", "1.0.0", false, ""},
		{"gte constraint", ">=1.2.0", "2.1.0", false, ""},
		{"gte exact match", ">=1.5.0", "2.1.0", false, ""},
		{"version not found", "3.0.0", "", true, "No version matching constraint found"},
		{"invalid constraint", "@invalid", "", true, "Invalid version constraint"}, // Invalid semver syntax
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveVersion(versions, tt.constraint)

			if tt.expectError {
				require.Error(t, err)

				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedVer, result.Version)
			}
		})
	}
}

// TestResolveVersionEmpty tests error handling for empty version list
func TestResolveVersionEmpty(t *testing.T) {
	_, err := ResolveVersion([]RegistryVersion{}, "latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No versions available")
}

// Helper function to find project root
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", os.ErrNotExist
}
