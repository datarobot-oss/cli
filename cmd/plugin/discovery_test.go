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
	"os"
	"path/filepath"
	"testing"
	"time"

	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPluginDiscoveryTimeoutRoot(t *testing.T) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "dr"}
	root.PersistentFlags().Duration(
		internalPlugin.DiscoveryTimeoutKey,
		internalPlugin.DefaultDiscoveryTimeout,
		"",
	)

	return root
}

func TestPluginDiscoveryTimeout_FlagBeatsEnv(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "9s")

	root := newPluginDiscoveryTimeoutRoot(t)
	require.NoError(t, root.PersistentFlags().Set(internalPlugin.DiscoveryTimeoutKey, "0s"))

	assert.Equal(t, time.Duration(0), pluginDiscoveryTimeout(root))
}

func TestPluginDiscoveryTimeout_UsesEnvBeforeConfigIsRead(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "25ms")

	root := newPluginDiscoveryTimeoutRoot(t)

	assert.Equal(t, 25*time.Millisecond, pluginDiscoveryTimeout(root))
}

func TestPluginDiscoveryTimeout_InvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "not-a-duration")

	root := newPluginDiscoveryTimeoutRoot(t)

	assert.Equal(t, internalPlugin.DefaultDiscoveryTimeout, pluginDiscoveryTimeout(root))
}

// This test consumes the package-global plugin registry (PrimeCache/GetPlugins
// use sync.Once); no other test in this package reads that registry.
func TestRegisterPluginCommands_DisabledSeedsEmptyLazyCache(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_PLUGIN_DISCOVERY_TIMEOUT", "9s")

	tempDir := t.TempDir()
	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tempDir)

	// A valid plugin on PATH: if the lazy path re-discovered despite the
	// startup flag, the 9s env value would find it and this test would fail.
	pluginPath := filepath.Join(tempDir, "dr-fakeplugin")
	script := "#!/bin/sh\necho '{\"name\":\"fakeplugin\",\"version\":\"1.0.0\"}'\n"
	require.NoError(t, os.WriteFile(pluginPath, []byte(script), 0o755))
	t.Setenv("PATH", tempDir)

	root := newPluginDiscoveryTimeoutRoot(t)
	require.NoError(t, root.PersistentFlags().Set(internalPlugin.DiscoveryTimeoutKey, "0s"))

	RegisterPluginCommands(root)

	plugins, conflicts := internalPlugin.GetPlugins()
	assert.Empty(t, plugins, "lazy discovery must not re-run after startup disabled it")
	assert.Empty(t, conflicts)
}
