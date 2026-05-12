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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".wapi"), 0o755))

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

func TestRollback_MissingFileNoop(t *testing.T) {
	dir := setupProject(t)

	r, err := NewRollback(dir)
	require.NoError(t, err)

	require.NoError(t, r.Backup("never-existed.py"))

	require.NoError(t, r.Discard())
}

func TestRollback_AlreadyExists(t *testing.T) {
	dir := setupProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".wapi", ".rollback"), 0o755))

	_, err := NewRollback(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestRestoreStaleIfPresent(t *testing.T) {
	dir := setupProject(t)
	writeProjectFile(t, dir, "agent.py", "WAS-BROKEN")

	rollDir := filepath.Join(dir, ".wapi", ".rollback")
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
