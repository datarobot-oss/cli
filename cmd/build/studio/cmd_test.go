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

package studio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmdMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "studio", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.Flags().Lookup("yes"), "studio should expose the --yes flag")
}

func TestInstallPlugin_UnknownName(t *testing.T) {
	registry := &plugin.PluginRegistry{Plugins: map[string]plugin.RegistryPlugin{}}

	err := installPlugin(Cmd(), registry, "https://example.invalid", "xp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `plugin "xp" not found`)
}

func TestInstallPlugin_NoVersions(t *testing.T) {
	registry := &plugin.PluginRegistry{
		Plugins: map[string]plugin.RegistryPlugin{
			"xp": {Name: "xp"},
		},
	}

	err := installPlugin(Cmd(), registry, "https://example.invalid", "xp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve the latest version")
}

// TestEnsureStudioPlugins_AllInstalled primes the process-wide discovery
// cache with both plugins so ensureStudioPlugins resolves xp without touching
// the network or the filesystem. PrimeCache is once-per-process, so negative
// findInstalled lookups are asserted here too instead of in separate tests.
func TestEnsureStudioPlugins_AllInstalled(t *testing.T) {
	plugin.PrimeCache([]plugin.DiscoveredPlugin{
		{Manifest: manifestNamed("xp"), Executable: "/fake/xp.sh"},
		{Manifest: manifestNamed("opencode"), Executable: "/fake/opencode.sh"},
	}, nil)

	xp, err := ensureStudioPlugins(Cmd())

	require.NoError(t, err)
	require.NotNil(t, xp)
	assert.Equal(t, "xp", xp.Manifest.Name)
	assert.Equal(t, "/fake/xp.sh", xp.Executable)

	assert.Nil(t, findInstalled("no-such-plugin"), "an unknown plugin should not resolve")
}

// TestResolveInstalled_ManagedFallback verifies that a plugin absent from the
// discovery cache is still resolved from the managed plugins directory, so
// disabling discovery does not trigger a reinstall.
func TestResolveInstalled_ManagedFallback(t *testing.T) {
	tmpXDG := t.TempDir()

	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)

	pluginDir := filepath.Join(tmpXDG, "datarobot", "plugins", "managed-only")

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	manifest := `{"name":"managed-only","version":"1.0.0","scripts":{"posix":"run.sh","windows":"run.ps1"}}`

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "run.ps1"), []byte(""), 0o644))

	p := resolveInstalled("managed-only")

	require.NotNil(t, p, "a managed plugin should resolve even when absent from the discovery cache")
	assert.Equal(t, "managed-only", p.Manifest.Name)
}

func manifestNamed(name string) plugin.PluginManifest {
	return plugin.PluginManifest{
		BasicPluginManifest: plugin.BasicPluginManifest{Name: name, Version: "1.0.0"},
	}
}
