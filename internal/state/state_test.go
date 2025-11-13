// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
	t.Run("returns local path when .datarobot/state exists", func(t *testing.T) {
		// Create temporary directory with .datarobot/state
		tmpDir := t.TempDir()

		// Resolve symlinks (important for macOS where /var -> /private/var)
		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		localStateDir := filepath.Join(tmpDir, ".datarobot", "state")

		err = os.MkdirAll(localStateDir, 0o755)
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

		expected := filepath.Join(tmpDir, ".datarobot", "state", "currentstate.yml")
		assert.Equal(t, expected, statePath)
	})

	t.Run("returns XDG_STATE_HOME path when set and local doesn't exist", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		// Set XDG_STATE_HOME
		originalXDG := os.Getenv("XDG_STATE_HOME")

		defer func() {
			err := os.Setenv("XDG_STATE_HOME", originalXDG)
			require.NoError(t, err)
		}()

		err := os.Setenv("XDG_STATE_HOME", tmpDir)
		require.NoError(t, err)

		// Get state path (from a directory without .datarobot/state)
		anotherTmpDir := t.TempDir()
		originalWd, err := os.Getwd()
		require.NoError(t, err)

		defer func() {
			err := os.Chdir(originalWd)
			require.NoError(t, err)
		}()

		err = os.Chdir(anotherTmpDir)
		require.NoError(t, err)

		statePath, err := GetStatePath()
		require.NoError(t, err)

		expected := filepath.Join(tmpDir, "dr", "currentstate.yml")
		assert.Equal(t, expected, statePath)
	})
}

func TestLoadSave(t *testing.T) {
	t.Run("Save creates file and Load reads it back", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "state")
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
			LastSuccessfulRun: now,
			CLIVersion:        "1.0.0",
		}

		err = Save(originalState)
		require.NoError(t, err)

		// Load state back
		loadedState, err := Load()
		require.NoError(t, err)
		require.NotNil(t, loadedState)

		assert.Equal(t, originalState.CLIVersion, loadedState.CLIVersion)
		assert.Equal(t, now.Unix(), loadedState.LastSuccessfulRun.Unix())
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
	localStateDir := filepath.Join(tmpDir, ".datarobot", "state")
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

	err = UpdateAfterSuccessfulRun("1.2.3")
	require.NoError(t, err)

	afterUpdate := time.Now().UTC()

	// Load and verify
	state, err := Load()
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, "1.2.3", state.CLIVersion)
	assert.True(t, state.LastSuccessfulRun.After(beforeUpdate) || state.LastSuccessfulRun.Equal(beforeUpdate))
	assert.True(t, state.LastSuccessfulRun.Before(afterUpdate) || state.LastSuccessfulRun.Equal(afterUpdate))
}
