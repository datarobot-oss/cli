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
		statePath := getStatePath(tmpDir)

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
		lastStart := time.Now().UTC().Truncate(time.Second)

		originalState := state{
			fullPath:   getStatePath(tmpDir),
			LastStart:  &lastStart,
			CLIVersion: "1.0.0",
		}

		err = originalState.save()
		require.NoError(t, err)

		// Load state back
		loadedState, err := load(tmpDir)
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
		loadedState, err := load(tmpDir)
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

	err = UpdateAfterSuccessfulRun(tmpDir)
	require.NoError(t, err)

	afterUpdate := time.Now().UTC()

	// Load and verify
	loadedState, err := load(tmpDir)
	require.NoError(t, err)

	assert.NotEmpty(t, loadedState.CLIVersion)
	assert.True(t, loadedState.LastStart.After(beforeUpdate) || loadedState.LastStart.Equal(beforeUpdate))
	assert.True(t, loadedState.LastStart.Before(afterUpdate) || loadedState.LastStart.Equal(afterUpdate))
}

func TestUpdateAfterSuccessDepsCheck(t *testing.T) {
	tmpDir := t.TempDir()
	localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
	err := os.MkdirAll(localStateDir, 0o755)
	require.NoError(t, err)

	beforeUpdate := time.Now().UTC()

	err = UpdateAfterSuccessDepsCheck(tmpDir)
	require.NoError(t, err)

	afterUpdate := time.Now().UTC()

	loadedState, err := load(tmpDir)
	require.NoError(t, err)

	require.NotNil(t, loadedState.LastSuccessDepsCheck)
	assert.NotEmpty(t, loadedState.CLIVersion)
	assert.True(t, loadedState.LastSuccessDepsCheck.After(beforeUpdate) || loadedState.LastSuccessDepsCheck.Equal(beforeUpdate))
	assert.True(t, loadedState.LastSuccessDepsCheck.Before(afterUpdate) || loadedState.LastSuccessDepsCheck.Equal(afterUpdate))
}

func TestHasRecentSuccessDepsCheck(t *testing.T) {
	t.Run("returns false when no state file exists", func(t *testing.T) {
		assert.False(t, HasRecentSuccessDepsCheck(t.TempDir()))
	})

	t.Run("returns false when LastSuccessDepsCheck is nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		err = UpdateAfterSuccessfulRun(tmpDir)
		require.NoError(t, err)

		assert.False(t, HasRecentSuccessDepsCheck(tmpDir))
	})

	t.Run("returns true when LastSuccessDepsCheck is within 24 hours", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		err = UpdateAfterSuccessDepsCheck(tmpDir)
		require.NoError(t, err)

		assert.True(t, HasRecentSuccessDepsCheck(tmpDir))
	})

	t.Run("returns false when LastSuccessDepsCheck is older than 24 hours", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		stale := time.Now().UTC().Add(-25 * time.Hour)
		s := state{
			fullPath:             filepath.Join(tmpDir, ".datarobot", "cli", "state.yaml"),
			LastSuccessDepsCheck: &stale,
		}

		err = s.save()
		require.NoError(t, err)

		assert.False(t, HasRecentSuccessDepsCheck(tmpDir))
	})
}

func TestUpdateAfterSuccessDepsCheck_PreservesOtherFields(t *testing.T) {
	tmpDir := t.TempDir()
	localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
	err := os.MkdirAll(localStateDir, 0o755)
	require.NoError(t, err)

	// Write initial state with a LastStart timestamp.
	err = UpdateAfterSuccessfulRun(tmpDir)
	require.NoError(t, err)

	stateBefore, err := load(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, stateBefore.LastStart)

	// Now run deps check — LastStart must survive.
	err = UpdateAfterSuccessDepsCheck(tmpDir)
	require.NoError(t, err)

	stateAfter, err := load(tmpDir)
	require.NoError(t, err)

	require.NotNil(t, stateAfter.LastSuccessDepsCheck)
	require.NotNil(t, stateAfter.LastStart)
	assert.Equal(t, stateBefore.LastStart.Unix(), stateAfter.LastStart.Unix())
}

func TestUpdateAfterTemplatesSetup(t *testing.T) {
	t.Run("saves template name, ID and timestamp", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		beforeUpdate := time.Now().UTC()

		err = UpdateAfterTemplatesSetup(tmpDir, "my-awesome-template", "tmpl-abc123")
		require.NoError(t, err)

		afterUpdate := time.Now().UTC()

		loadedState, err := load(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "my-awesome-template", loadedState.TemplateName)
		assert.Equal(t, "tmpl-abc123", loadedState.TemplateID)
		assert.NotEmpty(t, loadedState.CLIVersion)
		require.NotNil(t, loadedState.LastTemplatesSetup)
		assert.True(t, loadedState.LastTemplatesSetup.After(beforeUpdate) || loadedState.LastTemplatesSetup.Equal(beforeUpdate))
		assert.True(t, loadedState.LastTemplatesSetup.Before(afterUpdate) || loadedState.LastTemplatesSetup.Equal(afterUpdate))
	})

	t.Run("preserves other fields when updating", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		err = UpdateAfterSuccessfulRun(tmpDir)
		require.NoError(t, err)

		stateBefore, err := load(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, stateBefore.LastStart)

		err = UpdateAfterTemplatesSetup(tmpDir, "preserved-template", "tmpl-xyz999")
		require.NoError(t, err)

		stateAfter, err := load(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, "preserved-template", stateAfter.TemplateName)
		assert.Equal(t, "tmpl-xyz999", stateAfter.TemplateID)
		require.NotNil(t, stateAfter.LastStart)
		assert.Equal(t, stateBefore.LastStart.Unix(), stateAfter.LastStart.Unix())
	})
}

func TestGetTemplateInfo(t *testing.T) {
	t.Run("returns both name and ID when state exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		err = UpdateAfterTemplatesSetup(tmpDir, "my-template", "tmpl-id-001")
		require.NoError(t, err)

		name, id := GetTemplateInfo(tmpDir)

		assert.Equal(t, "my-template", name)
		assert.Equal(t, "tmpl-id-001", id)
	})

	t.Run("returns empty strings when no state file exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		name, id := GetTemplateInfo(tmpDir)

		assert.Empty(t, name)
		assert.Empty(t, id)
	})

	t.Run("returns empty strings when template info not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")
		err := os.MkdirAll(localStateDir, 0o755)
		require.NoError(t, err)

		err = UpdateAfterSuccessfulRun(tmpDir)
		require.NoError(t, err)

		name, id := GetTemplateInfo(tmpDir)

		assert.Empty(t, name)
		assert.Empty(t, id)
	})
}
