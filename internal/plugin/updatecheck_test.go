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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRegistry(name, version string) PluginRegistry {
	return PluginRegistry{
		Version: "1",
		Plugins: map[string]RegistryPlugin{
			name: {
				Name:        name,
				Description: "test plugin",
				Versions: []RegistryVersion{
					{Version: version, URL: "test/test.tar.xz", SHA256: "abc123"},
				},
			},
		},
	}
}

func serveRegistry(t *testing.T, registry PluginRegistry) *httptest.Server {
	t.Helper()

	data, err := json.Marshal(registry)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))

	t.Cleanup(srv.Close)

	return srv
}

func TestCheckForUpdate(t *testing.T) {
	t.Run("returns nil when check interval is 0 (disabled)", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", time.Duration(0))
		defer viperx.Set("plugin-update-check-interval", nil)

		result := CheckForUpdate("assist", "0.1.0", "http://localhost/index.json")

		assert.Nil(t, result)
	})

	t.Run("returns nil when cooldown has not elapsed", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 24*time.Hour)
		defer viperx.Set("plugin-update-check-interval", nil)

		// Set a recent check timestamp
		state.SetLastPluginCheck("assist")

		result := CheckForUpdate("assist", "0.1.0", "http://localhost/index.json")

		assert.Nil(t, result)
	})

	t.Run("returns nil when network is unreachable", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		// Use a URL that will fail immediately
		result := CheckForUpdate("assist", "0.1.0", "http://192.0.2.1:1/index.json")

		assert.Nil(t, result)
	})

	t.Run("returns nil when plugin is already at latest version", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.1.13")
		srv := serveRegistry(t, registry)

		result := CheckForUpdate("assist", "0.1.13", srv.URL+"/index.json")

		assert.Nil(t, result)
	})

	t.Run("returns update result when newer version is available", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.2.0")
		srv := serveRegistry(t, registry)

		result := CheckForUpdate("assist", "0.1.13", srv.URL+"/index.json")

		require.NotNil(t, result)
		assert.Equal(t, "assist", result.PluginName)
		assert.Equal(t, "0.1.13", result.InstalledVersion)
		assert.Equal(t, "0.2.0", result.LatestVersion.Version)
	})

	t.Run("returns nil when plugin not in registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("other-plugin", "1.0.0")
		srv := serveRegistry(t, registry)

		result := CheckForUpdate("assist", "0.1.0", srv.URL+"/index.json")

		assert.Nil(t, result)
	})

	t.Run("returns nil when installed version is higher than latest", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.1.0")
		srv := serveRegistry(t, registry)

		result := CheckForUpdate("assist", "0.2.0", srv.URL+"/index.json")

		assert.Nil(t, result)
	})

	t.Run("handles non-semver installed version via string comparison", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.2.0")
		srv := serveRegistry(t, registry)

		// "unknown" is not valid semver, but != "0.2.0", so should return an update
		result := CheckForUpdate("assist", "unknown", srv.URL+"/index.json")

		require.NotNil(t, result)
		assert.Equal(t, "0.2.0", result.LatestVersion.Version)
	})

	t.Run("returns nil for non-semver when versions match as strings", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.2.0")
		srv := serveRegistry(t, registry)

		// Non-semver but same string → no update
		result := CheckForUpdate("assist", "0.2.0", srv.URL+"/index.json")

		assert.Nil(t, result)
	})

	t.Run("sets cooldown after successful registry fetch even when already up-to-date", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		registry := newTestRegistry("assist", "0.1.13")
		srv := serveRegistry(t, registry)

		// Confirm cooldown not yet set
		assert.True(t, state.GetLastPluginCheck("assist").IsZero())

		result := CheckForUpdate("assist", "0.1.13", srv.URL+"/index.json")

		assert.Nil(t, result)
		// Cooldown must be recorded so the next invocation skips the network call
		assert.False(t, state.GetLastPluginCheck("assist").IsZero())
	})

	t.Run("returns nil when registry returns HTTP error", func(t *testing.T) {
		viperx.Set("plugin-update-check-interval", 1*time.Millisecond)
		defer viperx.Set("plugin-update-check-interval", nil)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

		t.Cleanup(srv.Close)

		result := CheckForUpdate("assist", "0.1.0", srv.URL+"/index.json")

		assert.Nil(t, result)
	})
}
