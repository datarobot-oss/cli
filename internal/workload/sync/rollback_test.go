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

package sync

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(wapi.Dir(dir), 0o755))

	return dir
}

func writeProjectFile(t *testing.T, dir, rel, body string) {
	t.Helper()

	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func TestRollback_BackupAndRestore(t *testing.T) {
	dir := setupProject(t)
	writeProjectFile(t, dir, "agent.py", "original")
	writeProjectFile(t, dir, "utils/helper.py", "old helper")

	r, err := NewRollback(dir)
	require.NoError(t, err)

	require.NoError(t, r.Backup("agent.py"))
	require.NoError(t, r.Backup("utils/helper.py"))

	// Simulate Phase 5 modifying the working tree.
	writeProjectFile(t, dir, "agent.py", "BROKEN")
	writeProjectFile(t, dir, "utils/helper.py", "BROKEN")
	writeProjectFile(t, dir, "newly-created.txt", "leftover")
	r.TrackCreated(filepath.Join(dir, "newly-created.txt"))

	require.NoError(t, r.Restore())

	got, err := os.ReadFile(filepath.Join(dir, "agent.py"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))

	got, err = os.ReadFile(filepath.Join(dir, "utils/helper.py"))
	require.NoError(t, err)
	assert.Equal(t, "old helper", string(got))

	_, err = os.Stat(filepath.Join(dir, "newly-created.txt"))
	assert.ErrorIs(t, err, os.ErrNotExist, "tracked-created file should be removed")
}

func TestRollback_BackupAndRestore_PreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	dir := setupProject(t)
	writeProjectFile(t, dir, "entrypoint.sh", "#!/bin/sh\n")

	scriptPath := filepath.Join(dir, "entrypoint.sh")
	require.NoError(t, os.Chmod(scriptPath, 0o755))

	r, err := NewRollback(dir)
	require.NoError(t, err)

	require.NoError(t, r.Backup("entrypoint.sh"))

	// Simulate Phase 5 overwriting the file with a different mode.
	require.NoError(t, os.WriteFile(scriptPath, []byte("BROKEN"), 0o644))

	require.NoError(t, r.Restore())

	info, err := os.Stat(scriptPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestRollback_MissingFileNoop(t *testing.T) {
	dir := setupProject(t)

	r, err := NewRollback(dir)
	require.NoError(t, err)

	require.NoError(t, r.Backup("never-existed.py"))

	require.NoError(t, r.Discard())
}

func TestRollback_AlreadyExists(t *testing.T) {
	dir := setupProject(t)
	require.NoError(t, os.Mkdir(wapi.RollbackDir(dir), 0o755))

	_, err := NewRollback(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRestoreStaleIfPresent(t *testing.T) {
	dir := setupProject(t)
	writeProjectFile(t, dir, "agent.py", "WAS-BROKEN")

	rollDir := wapi.RollbackDir(dir)
	require.NoError(t, os.MkdirAll(rollDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(rollDir, "agent.py"), []byte("intact"), 0o644))

	restored, err := RestoreStaleIfPresent(dir)
	require.NoError(t, err)
	assert.True(t, restored)

	got, err := os.ReadFile(filepath.Join(dir, "agent.py"))
	require.NoError(t, err)
	assert.Equal(t, "intact", string(got))

	_, err = os.Stat(rollDir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRestoreStaleIfPresent_None(t *testing.T) {
	dir := setupProject(t)

	restored, err := RestoreStaleIfPresent(dir)
	require.NoError(t, err)
	assert.False(t, restored)
}

// A sync that crashed before the state directory moved leaves its backups
// under the legacy path. If the project also has a current state directory,
// EnsureMigrated refuses to merge the two, so nothing relocates those
// backups; recovery has to find them there or the user's overwritten files
// are lost.
func TestRestoreStaleIfPresent_RecoversFromLegacyLocation(t *testing.T) {
	dir := t.TempDir()

	// Current state dir exists, so EnsureMigrated would leave the legacy one
	// in place rather than merging.
	require.NoError(t, os.MkdirAll(wapi.Dir(dir), 0o755))

	legacyRollback := filepath.Join(dir, wapi.LegacyDirName, wapi.RollbackDirName)
	require.NoError(t, os.MkdirAll(legacyRollback, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyRollback, "agent.py"), []byte("original\n"), 0o600))

	// The working tree currently holds the half-applied server copy.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agent.py"), []byte("clobbered\n"), 0o600))

	restored, err := RestoreStaleIfPresent(dir)
	require.NoError(t, err)
	assert.True(t, restored, "a stranded legacy rollback must be reported as restored")

	body, err := os.ReadFile(filepath.Join(dir, "agent.py"))
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(body), "the user's file must come back")

	assert.NoDirExists(t, legacyRollback, "the tree is consumed once applied")
}

func TestRestoreStaleIfPresent_NoTreeAnywhere(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(wapi.Dir(dir), 0o755))

	restored, err := RestoreStaleIfPresent(dir)
	require.NoError(t, err)
	assert.False(t, restored)
}
