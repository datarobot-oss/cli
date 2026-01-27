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
		statePath, err := getStatePath()
		require.NoError(t, err)

		expected := filepath.Join(tmpDir, ".datarobot", "cli", "state.yaml")
		assert.Equal(t, expected, statePath)
	})
}

func TestLoadSave(t *testing.T) {
	t.Run("save() creates file and load() reads it back", func(t *testing.T) {
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
		originalState := state{
			LastStart:  time.Now().UTC().Truncate(time.Second),
			CLIVersion: "1.0.0",
		}

		err = originalState.save()
		require.NoError(t, err)

		// Load state back
		loadedState, err := load()
		require.NoError(t, err)

		assert.Equal(t, originalState.CLIVersion, loadedState.CLIVersion)
		assert.Equal(t, originalState.LastStart.Unix(), loadedState.LastStart.Unix())
	})

	t.Run("load() returns zero value when file doesn't exist", func(t *testing.T) {
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
		loadedState, err := load()
		require.NoError(t, err)
		assert.Nil(t, loadedState.LastTemplatesSetup)
		assert.Nil(t, loadedState.LastDotenvSetup)
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
	loadedState, err := load()
	require.NoError(t, err)

	assert.NotEmpty(t, loadedState.CLIVersion)
	assert.True(t, loadedState.LastStart.After(beforeUpdate) || loadedState.LastStart.Equal(beforeUpdate))
	assert.True(t, loadedState.LastStart.Before(afterUpdate) || loadedState.LastStart.Equal(afterUpdate))
}
