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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/log"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogOutput redirects os.Stderr to a pipe, reinitializes the stderr
// logger, runs fn, then returns everything written during fn's execution.
// Mirrors the equivalent helper in internal/plugin/discover_test.go.
func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = w

	log.StartStderr()

	fn()

	w.Close()

	os.Stderr = origStderr

	t.Cleanup(log.StopStderr)

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	r.Close()

	return buf.String()
}

func TestIsManagedPlugin(t *testing.T) {
	t.Run("returns true for plugin in primary XDG dir", func(t *testing.T) {
		tmpXDG := t.TempDir()

		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)

		pluginPath := filepath.Join(tmpXDG, "datarobot", "plugins", "my-plugin", "scripts", "run.sh")

		assert.True(t, isManagedPlugin(pluginPath))
	})

	t.Run("returns true for plugin in XDG_CONFIG_DIRS", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpXDG := t.TempDir()
		tmpConfigDir := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)
		testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", tmpConfigDir)

		configDirPath := filepath.Join(tmpConfigDir, "datarobot", "plugins", "my-plugin", "scripts", "run.sh")

		assert.True(t, isManagedPlugin(configDirPath))
	})

	t.Run("returns false for plugin on PATH outside managed dirs", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpXDG := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)

		pathPlugin := filepath.Join("/usr", "local", "bin", "dr-myplugin")

		assert.False(t, isManagedPlugin(pathPlugin))
	})
}

// TestReportDiscoveryConflicts verifies routine discovery reporting stays
// silent (Info-level) about CLI version incompatibility skips while still
// warning about name conflicts, per the spec's "Silent on routine command
// discovery" requirement. Version-incompatibility skips surface only through
// `dr plugin list` / `dr plugin version <name>`, which call
// internalPlugin.LogConflicts directly (unfiltered) instead of this helper.
func TestReportDiscoveryConflicts(t *testing.T) {
	t.Run("name conflicts are logged at Warn", func(t *testing.T) {
		output := captureLogOutput(t, func() {
			reportDiscoveryConflicts([]internalPlugin.PluginConflict{
				{Name: "widget", Path: "/usr/local/bin/dr-widget"},
			})
		})

		assert.Contains(t, output, "widget")
		assert.Contains(t, output, "WARN")
	})

	t.Run("version-incompatibility skips produce no Info-level output", func(t *testing.T) {
		output := captureLogOutput(t, func() {
			reportDiscoveryConflicts([]internalPlugin.PluginConflict{
				{
					Name:   "gadget",
					Path:   "/usr/local/bin/dr-gadget",
					Reason: internalPlugin.SkipReasonVersionIncompatible,
					Detail: "requires dr >= 2.0.0 (running 1.9.0); run 'dr self update'",
				},
			})
		})

		assert.NotContains(t, output, "gadget",
			"a version-incompatibility skip must stay silent on routine command discovery")
		assert.Empty(t, output)
	})

	t.Run("mixed conflicts only surface the name conflict", func(t *testing.T) {
		output := captureLogOutput(t, func() {
			reportDiscoveryConflicts([]internalPlugin.PluginConflict{
				{Name: "widget", Path: "/usr/local/bin/dr-widget"},
				{
					Name:   "gadget",
					Path:   "/usr/local/bin/dr-gadget",
					Reason: internalPlugin.SkipReasonVersionIncompatible,
					Detail: "requires dr >= 2.0.0 (running 1.9.0); run 'dr self update'",
				},
			})
		})

		assert.Contains(t, output, "widget")
		assert.NotContains(t, output, "gadget")
	})
}
