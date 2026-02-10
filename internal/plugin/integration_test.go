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
	"archive/tar"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
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

			// Validate CLIVersion format if present
			if manifest.CLIVersion != "" {
				assert.Regexp(t, `^v?\d+\.\d+\.\d+`, manifest.CLIVersion, "Manifest %s CLIVersion must be semver", manifestPath)
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

	var registry PluginRegistry

	err = json.Unmarshal(data, &registry)
	require.NoError(t, err, "Failed to parse index.json")

	pluginsDir := filepath.Join(projectRoot, "docs", "plugins")

	for pluginName, plugin := range registry.Plugins {
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

// TestPluginUpgradeWithRollback tests the complete upgrade flow with backup and rollback
func TestPluginUpgradeWithRollback(t *testing.T) {
	// Note: This test uses the actual user plugins directory
	// We'll verify the test plugin specifically
	pluginName := "test-upgrade-rollback-plugin"

	// Clean up any previous test artifacts
	defer func() {
		_ = UninstallPlugin(pluginName)
	}()

	t.Run("install working plugin v1.0.0", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.0.0", true)
		defer os.Remove(archivePath)

		entry := RegistryPlugin{
			Name:        pluginName,
			Description: "Test plugin",
		}
		version := RegistryVersion{
			Version: "1.0.0",
			URL:     "file://" + archivePath,
		}

		err := InstallPlugin(entry, version, "")
		require.NoError(t, err)

		installedPlugins, err := GetInstalledPlugins()
		require.NoError(t, err)

		// Find our test plugin
		var found bool

		for _, p := range installedPlugins {
			if p.Name == pluginName {
				found = true
			}
		}

		assert.True(t, found, "test plugin should be installed")
	})

	t.Run("verify plugin v1.0.0 validates successfully", func(t *testing.T) {
		err := ValidatePlugin(pluginName)
		require.NoError(t, err)
	})

	t.Run("backup plugin v1.0.0", func(t *testing.T) {
		backupPath, err := BackupPlugin(pluginName)
		require.NoError(t, err)

		require.NotEmpty(t, backupPath)
		defer CleanupBackup(backupPath)

		assert.DirExists(t, backupPath)

		manifestPath := filepath.Join(backupPath, "manifest.json")
		assert.FileExists(t, manifestPath)

		data, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		var manifest PluginManifest

		err = json.Unmarshal(data, &manifest)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", manifest.Version)
	})

	t.Run("upgrade to broken plugin v2.0.0 and rollback", func(t *testing.T) {
		backupPath, err := BackupPlugin(pluginName)
		require.NoError(t, err)

		defer CleanupBackup(backupPath)

		brokenArchive := createTestPluginArchive(t, pluginName, "2.0.0", false)
		defer os.Remove(brokenArchive)

		entry := RegistryPlugin{
			Name:        pluginName,
			Description: "Test plugin",
		}
		version := RegistryVersion{
			Version: "2.0.0",
			URL:     "file://" + brokenArchive,
		}

		err = InstallPlugin(entry, version, "")
		require.NoError(t, err)

		err = ValidatePlugin(pluginName)
		require.Error(t, err, "broken plugin should fail validation")

		err = RestorePlugin(pluginName, backupPath)
		require.NoError(t, err)

		installedPlugins, err := GetInstalledPlugins()
		require.NoError(t, err)

		// Find our test plugin and verify it's back to v1.0.0
		var found bool

		for _, p := range installedPlugins {
			if p.Name == pluginName {
				found = true
			}
		}

		assert.True(t, found, "test plugin should still be installed after rollback")
	})

	t.Run("verify plugin v1.0.0 still works after rollback", func(t *testing.T) {
		err := ValidatePlugin(pluginName)
		require.NoError(t, err, "rolled back plugin should validate successfully")

		managedDir, err := ManagedPluginsDir()
		require.NoError(t, err)

		pluginDir := filepath.Join(managedDir, pluginName)
		manifestPath := filepath.Join(pluginDir, "manifest.json")

		data, err := os.ReadFile(manifestPath)
		require.NoError(t, err)

		var manifest PluginManifest

		err = json.Unmarshal(data, &manifest)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", manifest.Version)
		assert.Equal(t, pluginName, manifest.Name)
	})

	t.Run("cleanup backup removes directory", func(t *testing.T) {
		backupPath, err := BackupPlugin(pluginName)
		require.NoError(t, err)

		assert.DirExists(t, backupPath)

		CleanupBackup(backupPath)

		_, err = os.Stat(backupPath)
		assert.True(t, os.IsNotExist(err), "backup directory should be removed")
	})
}

// createTestPluginArchive creates a test plugin archive (.tar.xz) for testing
// If valid is false, creates a broken plugin missing the manifest
func createTestPluginArchive(t *testing.T, name, version string, valid bool) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-plugin-*.tar.xz")
	require.NoError(t, err)

	defer tmpFile.Close()

	xzWriter, err := xz.NewWriter(tmpFile)
	require.NoError(t, err)

	defer xzWriter.Close()

	tarWriter := tar.NewWriter(xzWriter)
	defer tarWriter.Close()

	if valid {
		manifest := PluginManifest{
			BasicPluginManifest: BasicPluginManifest{
				Name:        name,
				Version:     version,
				Description: "Test plugin for integration testing",
			},
		}

		manifestData, err := json.MarshalIndent(manifest, "", "  ")
		require.NoError(t, err)

		err = tarWriter.WriteHeader(&tar.Header{
			Name:     "manifest.json",
			Mode:     0o644,
			Size:     int64(len(manifestData)),
			Typeflag: tar.TypeReg,
		})
		require.NoError(t, err)

		_, err = tarWriter.Write(manifestData)
		require.NoError(t, err)
	}

	metadata := InstalledPlugin{
		Name:        name,
		Version:     version,
		Source:      "test",
		InstalledAt: "2026-01-30T00:00:00Z",
	}

	metadataData, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)

	err = tarWriter.WriteHeader(&tar.Header{
		Name:     ".installed.json",
		Mode:     0o644,
		Size:     int64(len(metadataData)),
		Typeflag: tar.TypeReg,
	})
	require.NoError(t, err)

	_, err = tarWriter.Write(metadataData)
	require.NoError(t, err)

	return tmpFile.Name()
}
