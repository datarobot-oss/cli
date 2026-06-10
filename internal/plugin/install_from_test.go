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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadNameFromArchive tests name extraction from .tar.xz archives.
func TestReadNameFromArchive(t *testing.T) {
	t.Run("valid archive returns manifest name", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, "my-plugin", "1.0.0", true)
		defer os.Remove(archivePath)

		name, err := readNameFromArchive(archivePath)

		require.NoError(t, err)
		assert.Equal(t, "my-plugin", name)
	})

	t.Run("archive missing manifest.json returns error", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, "broken", "1.0.0", false)
		defer os.Remove(archivePath)

		_, err := readNameFromArchive(archivePath)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest.json")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		_, err := readNameFromArchive("/does/not/exist.tar.xz")

		require.Error(t, err)
	})
}

// TestInstallPluginFromFile tests installation from a local archive.
func TestInstallPluginFromFile(t *testing.T) {
	const pluginName = "test-install-from-file"

	defer func() { _ = UninstallPlugin(pluginName) }()

	t.Run("explicit name installs successfully", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.0.0", true)
		defer os.Remove(archivePath)

		resolvedName, err := InstallPluginFromFile(archivePath, pluginName)

		require.NoError(t, err)
		assert.Equal(t, pluginName, resolvedName)
		require.NoError(t, ValidatePlugin(pluginName))
	})

	t.Run("no name reads from manifest", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.1.0", true)
		defer os.Remove(archivePath)

		resolvedName, err := InstallPluginFromFile(archivePath, "")

		require.NoError(t, err)
		assert.Equal(t, pluginName, resolvedName)
		require.NoError(t, ValidatePlugin(pluginName))
	})

	t.Run("no name and no manifest returns error", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.0.0", false)
		defer os.Remove(archivePath)

		_, err := InstallPluginFromFile(archivePath, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin name")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		_, err := InstallPluginFromFile("/does/not/exist.tar.xz", pluginName)

		require.Error(t, err)
	})

	t.Run("metadata records local source", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "2.0.0", true)
		defer os.Remove(archivePath)

		_, err := InstallPluginFromFile(archivePath, pluginName)
		require.NoError(t, err)

		managedDir, err := ManagedPluginsDir()
		require.NoError(t, err)

		metaPath := filepath.Join(managedDir, pluginName, ".installed.json")
		assert.FileExists(t, metaPath)

		plugins, err := GetInstalledPlugins()
		require.NoError(t, err)

		var found bool

		for _, p := range plugins {
			if p.Name == pluginName {
				found = true

				assert.Equal(t, "local", p.Version)
				assert.NotEmpty(t, p.Source)
			}
		}

		assert.True(t, found, "installed plugin should appear in list")
	})
}

// TestInstallPluginFromURL tests installation from an HTTP URL.
func TestInstallPluginFromURL(t *testing.T) {
	const pluginName = "test-install-from-url"

	defer func() { _ = UninstallPlugin(pluginName) }()

	t.Run("explicit name installs successfully", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.0.0", true)
		defer os.Remove(archivePath)

		srv := serveFile(t, archivePath)
		defer srv.Close()

		resolvedName, err := InstallPluginFromURL(srv.URL+"/plugin.tar.xz", pluginName)

		require.NoError(t, err)
		assert.Equal(t, pluginName, resolvedName)
		require.NoError(t, ValidatePlugin(pluginName))
	})

	t.Run("no name reads from manifest", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.1.0", true)
		defer os.Remove(archivePath)

		srv := serveFile(t, archivePath)
		defer srv.Close()

		resolvedName, err := InstallPluginFromURL(srv.URL+"/plugin.tar.xz", "")

		require.NoError(t, err)
		assert.Equal(t, pluginName, resolvedName)
		require.NoError(t, ValidatePlugin(pluginName))
	})

	t.Run("no name and no manifest returns error", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "1.0.0", false)
		defer os.Remove(archivePath)

		srv := serveFile(t, archivePath)
		defer srv.Close()

		_, err := InstallPluginFromURL(srv.URL+"/plugin.tar.xz", "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin name")
	})

	t.Run("404 response returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		_, err := InstallPluginFromURL(srv.URL+"/missing.tar.xz", pluginName)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("metadata records source URL", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, pluginName, "2.0.0", true)
		defer os.Remove(archivePath)

		srv := serveFile(t, archivePath)
		defer srv.Close()

		pluginURL := srv.URL + "/plugin.tar.xz"

		_, err := InstallPluginFromURL(pluginURL, pluginName)
		require.NoError(t, err)

		plugins, err := GetInstalledPlugins()
		require.NoError(t, err)

		for _, p := range plugins {
			if p.Name == pluginName {
				assert.Equal(t, pluginURL, p.Source)
			}
		}
	})
}

// TestInstallPathTraversal verifies that unsafe plugin names are rejected before any
// filesystem operation, preventing directory traversal via --file, --url, or a
// crafted manifest.json.
func TestInstallPathTraversal(t *testing.T) {
	unsafe := []string{"../../etc/passwd", "../escape", "/absolute", "foo/bar", ".."}

	t.Run("InstallPluginFromFile rejects unsafe explicit name", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, "safe", "1.0.0", true)
		defer os.Remove(archivePath)

		for _, name := range unsafe {
			_, err := InstallPluginFromFile(archivePath, name)

			require.Errorf(t, err, "expected error for name %q", name)
		}
	})

	t.Run("InstallPluginFromURL rejects unsafe explicit name", func(t *testing.T) {
		archivePath := createTestPluginArchive(t, "safe", "1.0.0", true)
		defer os.Remove(archivePath)

		srv := serveFile(t, archivePath)
		defer srv.Close()

		for _, name := range unsafe {
			_, err := InstallPluginFromURL(srv.URL+"/plugin.tar.xz", name)

			require.Errorf(t, err, "expected error for name %q", name)
		}
	})

	t.Run("InstallPluginFromFile rejects unsafe name from manifest", func(t *testing.T) {
		for _, name := range unsafe {
			archivePath := createTestPluginArchive(t, name, "1.0.0", true)
			defer os.Remove(archivePath)

			_, err := InstallPluginFromFile(archivePath, "")

			require.Errorf(t, err, "expected error for manifest name %q", name)
		}
	})
}

// serveFile starts a test HTTP server that serves a single file for all requests.
func serveFile(t *testing.T, path string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	}))
}
