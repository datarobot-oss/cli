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

	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedPluginsDir(t *testing.T) {
	t.Run("respects XDG_CONFIG_HOME", func(t *testing.T) {
		testConfigHome := "/custom/config"
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", testConfigHome)

		dir, err := ManagedPluginsDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(testConfigHome, "datarobot", "plugins"), dir)
	})

	t.Run("falls back to ~/.config when XDG_CONFIG_HOME not set", func(t *testing.T) {
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", "")

		dir, err := ManagedPluginsDir()

		require.NoError(t, err)

		homeDir, _ := os.UserHomeDir()
		expected := filepath.Join(homeDir, ".config", "datarobot", "plugins")
		assert.Equal(t, expected, dir)
	})
}

func TestManagedPluginsDirs(t *testing.T) {
	t.Run("returns only primary dir when XDG_CONFIG_DIRS is not set", func(t *testing.T) {
		tmpHome := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", "")
		testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", "")

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 1)
		assert.Equal(t, filepath.Join(tmpHome, ".config", "datarobot", "plugins"), dirs[0])
	})

	t.Run("includes XDG_CONFIG_DIRS when set", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpXDG := t.TempDir()
		tmpConfigDir1 := t.TempDir()
		tmpConfigDir2 := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)
		testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", tmpConfigDir1+string(filepath.ListSeparator)+tmpConfigDir2)

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 3)
		assert.Equal(t, filepath.Join(tmpXDG, "datarobot", "plugins"), dirs[0], "primary XDG_CONFIG_HOME should be first")
		assert.Equal(t, filepath.Join(tmpConfigDir1, "datarobot", "plugins"), dirs[1])
		assert.Equal(t, filepath.Join(tmpConfigDir2, "datarobot", "plugins"), dirs[2])
	})

	t.Run("deduplicates when XDG_CONFIG_DIRS overlaps with primary dir", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpXDG := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)
		// Set XDG_CONFIG_DIRS to include the same path as XDG_CONFIG_HOME
		testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", tmpXDG)

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 1, "should deduplicate when XDG_CONFIG_DIRS contains primary dir")
		assert.Equal(t, filepath.Join(tmpXDG, "datarobot", "plugins"), dirs[0])
	})

	t.Run("handles single directory in XDG_CONFIG_DIRS", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpConfigDir := t.TempDir()

		t.Setenv("HOME", tmpHome)
		testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", "")
		testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", tmpConfigDir)

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 2)
		assert.Equal(t, filepath.Join(tmpHome, ".config", "datarobot", "plugins"), dirs[0])
		assert.Equal(t, filepath.Join(tmpConfigDir, "datarobot", "plugins"), dirs[1])
	})
}
