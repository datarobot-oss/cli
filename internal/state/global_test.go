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

package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLastPluginCheck(t *testing.T) {
	t.Run("returns zero time when no state file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		result := GetLastPluginCheck("my-plugin")

		assert.True(t, result.IsZero())
	})

	t.Run("returns zero time for unchecked plugin", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		// Write state with a different plugin
		SetLastPluginCheck("other-plugin")

		result := GetLastPluginCheck("my-plugin")

		assert.True(t, result.IsZero())
	})
}

func TestSetLastPluginCheck(t *testing.T) {
	t.Run("creates state file and records timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		before := time.Now().UTC().Add(-time.Second)

		SetLastPluginCheck("assist")

		after := time.Now().UTC().Add(time.Second)

		result := GetLastPluginCheck("assist")

		assert.False(t, result.IsZero())
		assert.True(t, result.After(before), "timestamp should be after test start")
		assert.True(t, result.Before(after), "timestamp should be before test end")
	})

	t.Run("preserves other plugins timestamps", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		SetLastPluginCheck("plugin-a")

		tsA := GetLastPluginCheck("plugin-a")

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		SetLastPluginCheck("plugin-b")

		// plugin-a's timestamp should be unchanged
		assert.Equal(t, tsA, GetLastPluginCheck("plugin-a"))
		assert.False(t, GetLastPluginCheck("plugin-b").IsZero())
	})

	t.Run("overwrites previous check timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		SetLastPluginCheck("assist")

		first := GetLastPluginCheck("assist")

		time.Sleep(10 * time.Millisecond)

		SetLastPluginCheck("assist")

		second := GetLastPluginCheck("assist")

		assert.True(t, second.After(first), "second check should have a later timestamp")
	})
}

func TestGlobalStateCorruptedFile(t *testing.T) {
	t.Run("handles corrupted state file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHomeDir(t, tmpDir)

		configDir := filepath.Join(tmpDir, ".config", "datarobot")

		err := os.MkdirAll(configDir, 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(configDir, "state.yaml"), []byte("{{invalid yaml"), 0o644)
		require.NoError(t, err)

		// Should not panic or error — returns zero time
		result := GetLastPluginCheck("my-plugin")

		assert.True(t, result.IsZero())

		// Should be able to write new state over corrupted file
		SetLastPluginCheck("my-plugin")

		result = GetLastPluginCheck("my-plugin")

		assert.False(t, result.IsZero())
	})
}
