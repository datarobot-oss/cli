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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedPluginsDir(t *testing.T) {
	t.Run("respects XDG_CONFIG_HOME", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")

		defer func() {
			if originalXDG != "" {
				os.Setenv("XDG_CONFIG_HOME", originalXDG)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}
		}()

		testConfigHome := "/custom/config"
		os.Setenv("XDG_CONFIG_HOME", testConfigHome)

		dir, err := ManagedPluginsDir()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(testConfigHome, "datarobot", "plugins"), dir)
	})

	t.Run("falls back to ~/.config when XDG_CONFIG_HOME not set", func(t *testing.T) {
		originalXDG := os.Getenv("XDG_CONFIG_HOME")

		defer func() {
			if originalXDG != "" {
				os.Setenv("XDG_CONFIG_HOME", originalXDG)
			} else {
				os.Unsetenv("XDG_CONFIG_HOME")
			}
		}()

		os.Unsetenv("XDG_CONFIG_HOME")

		dir, err := ManagedPluginsDir()

		require.NoError(t, err)

		homeDir, _ := os.UserHomeDir()
		expected := filepath.Join(homeDir, ".config", "datarobot", "plugins")
		assert.Equal(t, expected, dir)
	})
}

func TestManagedPluginsDirs(t *testing.T) {
	t.Run("returns only default dir when XDG_CONFIG_HOME is not set", func(t *testing.T) {
		tmpHome := t.TempDir()

		t.Setenv("HOME", tmpHome)
		t.Setenv("XDG_CONFIG_HOME", "")

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 1)
		assert.Equal(t, filepath.Join(tmpHome, ".config", "datarobot", "plugins"), dirs[0])
	})

	t.Run("returns XDG dir first then default fallback when XDG_CONFIG_HOME is set", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpXDG := t.TempDir()

		t.Setenv("HOME", tmpHome)
		t.Setenv("XDG_CONFIG_HOME", tmpXDG)

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 2)
		assert.Equal(t, filepath.Join(tmpXDG, "datarobot", "plugins"), dirs[0])
		assert.Equal(t, filepath.Join(tmpHome, ".config", "datarobot", "plugins"), dirs[1])
	})

	t.Run("deduplicates when XDG_CONFIG_HOME points to the same location as default", func(t *testing.T) {
		tmpHome := t.TempDir()

		t.Setenv("HOME", tmpHome)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

		dirs, err := ManagedPluginsDirs()

		require.NoError(t, err)
		require.Len(t, dirs, 1)
		assert.Equal(t, filepath.Join(tmpHome, ".config", "datarobot", "plugins"), dirs[0])
	})
}
