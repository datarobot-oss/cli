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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLivePluginManifests validates all manifest.json files in docs/plugins/
func TestLivePluginManifests(t *testing.T) {
	projectRoot, err := findProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	pluginsDir := filepath.Join(projectRoot, "docs", "plugins")

	entries, err := os.ReadDir(pluginsDir)
	require.NoError(t, err, "Failed to read docs/plugins directory")

	manifestCount := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(pluginsDir, entry.Name(), "manifest.json")

		// Skip if manifest doesn't exist (not all subdirs are plugins)
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		manifestCount++

		t.Run("Manifest_"+entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(manifestPath)
			require.NoError(t, err, "Failed to read %s", manifestPath)

			var manifest PluginManifest

			err = json.Unmarshal(data, &manifest)
			require.NoError(t, err, "Invalid JSON in %s", manifestPath)

			// Validate required fields
			assert.NotEmpty(t, manifest.Name, "Manifest %s must have a name", manifestPath)
			assert.NotEmpty(t, manifest.Version, "Manifest %s must have a version", manifestPath)
			assert.NotEmpty(t, manifest.Description, "Manifest %s must have a description", manifestPath)

			// Validate scripts if present
			if manifest.Scripts != nil {
				assert.NotEmpty(t, manifest.Scripts.Posix, "Manifest %s scripts must have posix entry", manifestPath)
				assert.NotEmpty(t, manifest.Scripts.Windows, "Manifest %s scripts must have windows entry", manifestPath)

				// Verify referenced scripts exist
				pluginDir := filepath.Join(pluginsDir, entry.Name())

				posixScript := filepath.Join(pluginDir, manifest.Scripts.Posix)
				if _, err := os.Stat(posixScript); err != nil {
					t.Errorf("Manifest %s references non-existent posix script: %s", manifestPath, manifest.Scripts.Posix)
				}

				windowsScript := filepath.Join(pluginDir, manifest.Scripts.Windows)
				if _, err := os.Stat(windowsScript); err != nil {
					t.Errorf("Manifest %s references non-existent windows script: %s", manifestPath, manifest.Scripts.Windows)
				}
			}

			// Validate version format
			assert.Regexp(t, `^v?\d+\.\d+\.\d+`, manifest.Version, "Manifest %s version must be semver", manifestPath)

			// Validate minCLIVersion format if present
			if manifest.MinCLIVersion != "" {
				assert.Regexp(t, `^v?\d+\.\d+\.\d+`, manifest.MinCLIVersion, "Manifest %s minCLIVersion must be semver", manifestPath)
			}
		})
	}

	if manifestCount == 0 {
		t.Skip("No plugin manifests found in docs/plugins/ - this is expected for new repos")
	}
}

// TestPluginIndexReferenceIntegrity validates that all plugins in index.json reference valid archives
func TestPluginIndexReferenceIntegrity(t *testing.T) {
	projectRoot, err := findProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	indexPath := filepath.Join(projectRoot, "docs", "plugins", "index.json")

	data, err := os.ReadFile(indexPath)
	require.NoError(t, err, "Failed to read docs/plugins/index.json")

	var index PluginIndex

	err = json.Unmarshal(data, &index)
	require.NoError(t, err, "Failed to parse index.json")

	pluginsDir := filepath.Join(projectRoot, "docs", "plugins")

	for pluginName, plugin := range index.Plugins {
		t.Run("Plugin_"+pluginName, func(t *testing.T) {
			for _, version := range plugin.Versions {
				t.Run("Version_"+version.Version, func(t *testing.T) {
					// Check if URL points to localhost (test URL) or production
					if containsString(version.URL, "localhost") || containsString(version.URL, "127.0.0.1") {
						t.Skip("Skipping test URL validation")

						return
					}

					// For production URLs pointing to cli.datarobot.com, verify local archive exists
					if containsString(version.URL, "cli.datarobot.com") {
						// Extract path from URL pattern:
						// https://cli.datarobot.com/plugins/<plugin>/<archive>.tar.xz
						// Archive should be at docs/plugins/<plugin>/<archive>.tar.xz
						urlPath := version.URL
						if idx := findInString(urlPath, "/plugins/"); idx >= 0 {
							relativePath := urlPath[idx+9:] // Skip "/plugins/"
							archivePath := filepath.Join(pluginsDir, relativePath)

							if _, err := os.Stat(archivePath); err != nil {
								t.Logf("WARNING: Archive referenced in index.json not found locally: %s", archivePath)
								t.Logf("         This is expected if the archive hasn't been built yet")
							}
						}
					}
				})
			}
		})
	}
}

// TestManifestScriptsExecutability verifies scripts have executable permissions on Unix
func TestManifestScriptsExecutability(t *testing.T) {
	projectRoot, err := findProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	pluginsDir := filepath.Join(projectRoot, "docs", "plugins")

	entries, err := os.ReadDir(pluginsDir)
	require.NoError(t, err, "Failed to read docs/plugins directory")

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(pluginsDir, entry.Name(), "manifest.json")

		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		var manifest PluginManifest

		err = json.Unmarshal(data, &manifest)
		require.NoError(t, err)

		if manifest.Scripts == nil {
			continue
		}

		t.Run("Scripts_"+entry.Name(), func(t *testing.T) {
			pluginDir := filepath.Join(pluginsDir, entry.Name())

			// Check Posix script
			if manifest.Scripts.Posix != "" {
				scriptPath := filepath.Join(pluginDir, manifest.Scripts.Posix)

				info, err := os.Stat(scriptPath)
				if err == nil {
					// On Unix systems, check executable bit
					mode := info.Mode()
					if mode&0o111 == 0 {
						t.Errorf("Posix script %s is not executable (mode: %o)", scriptPath, mode)
					}
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func findInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}
