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

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDotenvSetupTracking(t *testing.T) {
	t.Run("UpdateAfterDotenvSetup creates and updates state", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")

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

		err = UpdateAfterDotenvSetup(tmpDir)
		require.NoError(t, err)

		afterUpdate := time.Now().UTC()

		// Load and verify
		loadedState, err := load(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, loadedState)
		require.NotNil(t, loadedState.LastDotenvSetup)

		assert.True(t, loadedState.LastDotenvSetup.After(beforeUpdate) || loadedState.LastDotenvSetup.Equal(beforeUpdate))
		assert.True(t, loadedState.LastDotenvSetup.Before(afterUpdate) || loadedState.LastDotenvSetup.Equal(afterUpdate))
	})

	t.Run("UpdateAfterDotenvSetup preserves existing fields", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")

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
		err = UpdateAfterSuccessfulRun(tmpDir)
		require.NoError(t, err)

		// Update with dotenv setup
		err = UpdateAfterDotenvSetup(tmpDir)
		require.NoError(t, err)

		// Load and verify both fields are present
		loadedState, err := load(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, loadedState)

		assert.NotEmpty(t, loadedState.CLIVersion)
		assert.False(t, loadedState.LastStart.IsZero())
		assert.NotNil(t, loadedState.LastDotenvSetup)
	})

	t.Run("HasCompletedDotenvSetup returns true when setup completed in past", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")

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
		assert.False(t, HasCompletedDotenvSetup(tmpDir))

		// Update dotenv setup
		err = UpdateAfterDotenvSetup(tmpDir)
		require.NoError(t, err)

		// Now should be true
		assert.True(t, HasCompletedDotenvSetup(tmpDir))
	})

	t.Run("HasCompletedDotenvSetup returns false when never run", func(t *testing.T) {
		// Create temporary directory
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

		// Should be false with no state file
		assert.False(t, HasCompletedDotenvSetup(tmpDir))
	})

	t.Run("HasCompletedDotenvSetup returns false when force-interactive is true", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		tmpDir, err := filepath.EvalSymlinks(tmpDir)
		require.NoError(t, err)

		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")

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

		// Update dotenv setup to create state file
		err = UpdateAfterDotenvSetup(tmpDir)
		require.NoError(t, err)

		// Verify it returns true normally
		assert.True(t, HasCompletedDotenvSetup(tmpDir))

		// Set force-interactive flag
		oldValue := viper.GetBool("force-interactive")

		viper.Set("force-interactive", true)

		defer viper.Set("force-interactive", oldValue)

		// Now should return false even though state file exists
		assert.False(t, HasCompletedDotenvSetup(tmpDir))

		// Reset flag
		viper.Set("force-interactive", oldValue)

		// Should return true again
		assert.True(t, HasCompletedDotenvSetup(tmpDir))
	})
}
