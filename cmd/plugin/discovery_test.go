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
	"path/filepath"
	"testing"
	"time"

	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
