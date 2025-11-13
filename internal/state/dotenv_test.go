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

func TestDotenvSetupTracking(t *testing.T) {
	t.Run("UpdateAfterDotenvSetup creates and updates state", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

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

		// Update dotenv setup state
		beforeUpdate := time.Now().UTC()

		err = UpdateAfterDotenvSetup()
		require.NoError(t, err)

		afterUpdate := time.Now().UTC()

		// Load and verify
		state, err := Load()
		require.NoError(t, err)
		require.NotNil(t, state)
		require.NotNil(t, state.LastDotenvSetup)

		assert.True(t, state.LastDotenvSetup.After(beforeUpdate) || state.LastDotenvSetup.Equal(beforeUpdate))
		assert.True(t, state.LastDotenvSetup.Before(afterUpdate) || state.LastDotenvSetup.Equal(afterUpdate))
	})

	t.Run("UpdateAfterDotenvSetup preserves existing fields", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

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

		// Create initial state with dr start info
		err = UpdateAfterSuccessfulRun("1.0.0")
		require.NoError(t, err)

		// Update with dotenv setup
		err = UpdateAfterDotenvSetup()
		require.NoError(t, err)

		// Load and verify both fields are present
		state, err := Load()
		require.NoError(t, err)
		require.NotNil(t, state)

		assert.Equal(t, "1.0.0", state.CLIVersion)
		assert.False(t, state.LastSuccessfulRun.IsZero())
		assert.NotNil(t, state.LastDotenvSetup)
	})

	t.Run("HasCompletedDotenvSetup returns true when setup completed in past", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

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

		// Initially should be false
		assert.False(t, HasCompletedDotenvSetup())

		// Update dotenv setup
		err = UpdateAfterDotenvSetup()
		require.NoError(t, err)

		// Now should be true
		assert.True(t, HasCompletedDotenvSetup())
	})

	t.Run("HasCompletedDotenvSetup returns false when never run", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		// Override XDG_STATE_HOME to point to a non-existent location
		originalXDG := os.Getenv("XDG_STATE_HOME")
		nonExistentPath := filepath.Join(tmpDir, "nonexistent")

		defer func() {
			_ = os.Setenv("XDG_STATE_HOME", originalXDG)
		}()

		_ = os.Setenv("XDG_STATE_HOME", nonExistentPath)

		// Change to temp directory
		originalWd, err := os.Getwd()
		require.NoError(t, err)

		defer func() {
			err := os.Chdir(originalWd)
			require.NoError(t, err)
		}()

		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		// Should be false with no state file
		assert.False(t, HasCompletedDotenvSetup())
	})
}
