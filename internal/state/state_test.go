// Copyright 2025 DataRobot, Inc. and its affiliates.
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStatePath(t *testing.T) {
	t.Run("returns local .datarobot/cli path", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		// Resolve symlinks (important for macOS where /var -> /private/var)
		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		// Change to temp directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)

		defer func() {
			err := os.Chdir(originalWd)
			require.NoError(t, err)
		}()

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Get state path
		statePath, err := GetStatePath()
		require.NoError(t, err)

		expected := filepath.Join(tmpDir, ".datarobot", "cli", "state.yaml")
		assert.Equal(t, expected, statePath)
	})
}

func TestLoadSave(t *testing.T) {
	t.Run("Save creates file and Load reads it back", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		// Change to temp directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)

		defer func() {
			err := os.Chdir(originalWd)
			require.NoError(t, err)
		}()

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Create and save state
		now := time.Now().UTC().Truncate(time.Second)
		originalState := &State{
			LastStart:  now,
			CLIVersion: "1.0.0",
		}

		err = Save(originalState)
		require.NoError(t, err)

		// Load state back
		loadedState, err := Load()
		require.NoError(t, err)
		require.NotNil(t, loadedState)

		assert.Equal(t, originalState.CLIVersion, loadedState.CLIVersion)
		assert.Equal(t, now.Unix(), loadedState.LastStart.Unix())
	})

	t.Run("Load returns nil when file doesn't exist", func(t *testing.T) {
		// Create temporary directory without state file
		tmpDir := t.TempDir()

		// Change to temp directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)

		defer func() {
			err := os.Chdir(originalWd)
			require.NoError(t, err)
		}()

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Try to load non-existent state
		state, err := Load()
		require.NoError(t, err)
		assert.Nil(t, state)
	})
}

func TestUpdateAfterSuccessfulRun(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
	err := os.MkdirAll(localStateDir, 0o755)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)

	defer func() {
		err := os.Chdir(originalWd)
		require.NoError(t, err)
	}()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Update state
	beforeUpdate := time.Now().UTC()

	err = UpdateAfterSuccessfulRun()
	require.NoError(t, err)

	afterUpdate := time.Now().UTC()

	// Load and verify
	state, err := Load()
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.NotEmpty(t, state.CLIVersion)
	assert.True(t, state.LastStart.After(beforeUpdate) || state.LastStart.Equal(beforeUpdate))
	assert.True(t, state.LastStart.Before(afterUpdate) || state.LastStart.Equal(afterUpdate))
}
