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
	"errors"
	"io"
	"testing"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSeams replaces every seam with a safe default that fails the test when
// an unexpected dependency is reached, and restores the real implementations
// on cleanup. Tests override individual seams after calling it, so no test
// can ever run real discovery, hit the network, or install anything —
// regardless of the order the shuffler picks.
func stubSeams(t *testing.T) {
	t.Helper()

	origGetPlugins := getPlugins
	origLoadManaged := loadManagedPlugin
	origFetchRegistry := fetchRegistry
	origInstall := installRegistryEntry
	origCheckDeps := checkAndInstallDeps

	t.Cleanup(func() {
		getPlugins = origGetPlugins
		loadManagedPlugin = origLoadManaged
		fetchRegistry = origFetchRegistry
		installRegistryEntry = origInstall
		checkAndInstallDeps = origCheckDeps
	})

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return nil, nil
	}
	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		return nil, nil
	}
	fetchRegistry = func(string) (*plugin.PluginRegistry, string, error) {
		t.Fatal("unexpected registry fetch")

		return nil, "", nil
	}
	installRegistryEntry = func(plugin.RegistryPlugin, plugin.RegistryVersion, string) error {
		t.Fatal("unexpected plugin install")

		return nil
	}
	checkAndInstallDeps = func(string, func() bool, io.Writer) error {
		return nil
	}
}

func discovered(name, executable string) plugin.DiscoveredPlugin {
	return plugin.DiscoveredPlugin{
		Manifest: plugin.PluginManifest{
			BasicPluginManifest: plugin.BasicPluginManifest{Name: name, Version: "1.0.0"},
		},
		Executable: executable,
	}
}

// testRegistry returns a registry holding the xp and opencode entries with a
// single version each.
func testRegistry() *plugin.PluginRegistry {
	return &plugin.PluginRegistry{
		Plugins: map[string]plugin.RegistryPlugin{
			"xp": {
				Name:     "xp",
				Versions: []plugin.RegistryVersion{{Version: "2.2.0", URL: "xp/xp-2.2.0.tar.xz"}},
			},
			"opencode": {
				Name:     "opencode",
				Versions: []plugin.RegistryVersion{{Version: "0.1.11", URL: "opencode/opencode-0.1.11.tar.xz"}},
			},
		},
	}
}

func TestCmdMetadata(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "studio", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.Flags().Lookup("yes"), "studio should expose the --yes flag")
}

func TestEnsureStudioPlugins_AllInstalled(t *testing.T) {
	stubSeams(t)

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{
			discovered("xp", "/fake/xp.sh"),
			discovered("opencode", "/fake/opencode.sh"),
		}, nil
	}

	xp, err := ensureStudioPlugins(Cmd())

	require.NoError(t, err)
	require.NotNil(t, xp)
	assert.Equal(t, "xp", xp.Manifest.Name)
	assert.Equal(t, "/fake/xp.sh", xp.Executable)
}

func TestEnsureStudioPlugins_InstallsMissing(t *testing.T) {
	stubSeams(t)

	var installed []string

	installedSet := map[string]bool{}

	fetchRegistry = func(string) (*plugin.PluginRegistry, string, error) {
		return testRegistry(), "https://example.invalid/plugins", nil
	}
	installRegistryEntry = func(entry plugin.RegistryPlugin, version plugin.RegistryVersion, _ string) error {
		installed = append(installed, entry.Name+" "+version.Version)
		installedSet[entry.Name] = true

		return nil
	}
	// Resolves only what was actually installed, mirroring the real managed
	// lookup: before installation both plugins must read as missing.
	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		if !installedSet[name] {
			return nil, nil
		}

		p := discovered(name, "/managed/"+name+".sh")

		return &p, nil
	}

	xp, err := ensureStudioPlugins(Cmd())

	require.NoError(t, err)
	require.NotNil(t, xp)
	assert.Equal(t, "/managed/xp.sh", xp.Executable, "xp should be resolved from the managed dir after installation")
	assert.ElementsMatch(t, []string{"xp 2.2.0", "opencode 0.1.11"}, installed,
		"both missing plugins should be installed at their latest versions")
}

func TestEnsureStudioPlugins_InstallsOnlyOpenCode(t *testing.T) {
	stubSeams(t)

	var installed []string

	getPlugins = func() ([]plugin.DiscoveredPlugin, []plugin.PluginConflict) {
		return []plugin.DiscoveredPlugin{discovered("xp", "/fake/xp.sh")}, nil
	}
	fetchRegistry = func(string) (*plugin.PluginRegistry, string, error) {
		return testRegistry(), "https://example.invalid/plugins", nil
	}
	installRegistryEntry = func(entry plugin.RegistryPlugin, version plugin.RegistryVersion, _ string) error {
		installed = append(installed, entry.Name+" "+version.Version)

		return nil
	}

	xp, err := ensureStudioPlugins(Cmd())

	require.NoError(t, err)
	require.NotNil(t, xp)
	assert.Equal(t, "/fake/xp.sh", xp.Executable, "the already-installed xp should be reused")
	assert.Equal(t, []string{"opencode 0.1.11"}, installed, "only the missing opencode plugin should be installed")
}

func TestEnsureStudioPlugins_FetchRegistryFails(t *testing.T) {
	stubSeams(t)

	fetchRegistry = func(string) (*plugin.PluginRegistry, string, error) {
		return nil, "", errors.New("boom")
	}

	_, err := ensureStudioPlugins(Cmd())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch plugin registry")
}

func TestInstallPlugin_UnknownName(t *testing.T) {
	stubSeams(t)

	registry := &plugin.PluginRegistry{Plugins: map[string]plugin.RegistryPlugin{}}

	err := installPlugin(Cmd(), registry, "https://example.invalid", "xp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `plugin "xp" not found`)
}

func TestInstallPlugin_NoVersions(t *testing.T) {
	stubSeams(t)

	registry := &plugin.PluginRegistry{
		Plugins: map[string]plugin.RegistryPlugin{
			"xp": {Name: "xp"},
		},
	}

	err := installPlugin(Cmd(), registry, "https://example.invalid", "xp")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve the latest version")
}

// TestResolveInstalled_ManagedFallback verifies that a plugin absent from the
// discovery results is still resolved via the managed plugins lookup, so
// disabled discovery does not trigger a reinstall.
func TestResolveInstalled_ManagedFallback(t *testing.T) {
	stubSeams(t)

	loadManagedPlugin = func(name string) (*plugin.DiscoveredPlugin, error) {
		require.Equal(t, "xp", name)

		p := discovered("xp", "/managed/xp.sh")

		return &p, nil
	}

	p := resolveInstalled("xp")

	require.NotNil(t, p, "a managed plugin should resolve even when absent from discovery results")
	assert.Equal(t, "/managed/xp.sh", p.Executable)
}

// TestResolveInstalled_BrokenManagedInstall verifies that a managed plugin
// that fails to load is treated as not installed, so it gets reinstalled.
func TestResolveInstalled_BrokenManagedInstall(t *testing.T) {
	stubSeams(t)

	loadManagedPlugin = func(string) (*plugin.DiscoveredPlugin, error) {
		return nil, errors.New("missing script")
	}

	assert.Nil(t, resolveInstalled("xp"), "a broken managed installation should resolve to nil")
}
