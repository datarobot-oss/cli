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
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downloadFake serves canned bytes for DownloadFile; the rest of the FilesAPI
// surface comes from the erroring fakeFilesClient (engine_test.go).
type downloadFake struct {
	fakeFilesClient
	content map[string]string
}

func (d *downloadFake) DownloadFile(_, _, path string, w io.Writer) (string, int64, error) {
	n, _ := io.WriteString(w, d.content[path])

	return "", int64(n), nil
}

// writeLocal creates a file under dir and returns its absolute path.
func writeLocal(t *testing.T, dir, rel, content string) string {
	t.Helper()

	abs := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))

	return abs
}

// newTestRollback initializes the project state directory NewRollback needs
// and returns a fresh rollback for the sync engine's execute-phase helpers.
func newTestRollback(t *testing.T, dir string) *Rollback {
	t.Helper()

	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-test"}))

	rb, err := NewRollback(dir)
	require.NoError(t, err)

	return rb
}

// backupOverwrittenLocals moves every local file a remote-side action would
// destroy to a *.LOCAL copy, so nothing is lost without a recoverable backup.
func TestBackupOverwrittenLocals_BacksUpEveryDestructiveAction(t *testing.T) {
	dir := t.TempDir()

	modified := writeLocal(t, dir, "app.py", "local edits\n") // REMOTE_MODIFIED
	deleted := writeLocal(t, dir, "old.py", "still wanted\n") // REMOTE_DELETED
	conflicted := writeLocal(t, dir, "conf.py", "mine\n")     // CONFLICT

	rb := newTestRollback(t, dir)

	e := &Engine{
		projectDir: dir,
		nowFn:      staticNow,
		plan: &SyncPlan{
			Downloads: []FileAction{{Path: "app.py", Action: ActDownloadModify}},
			Deletes:   []FileAction{{Path: "old.py", Action: ActDownloadDelete}},
			Conflicts: []FileAction{{Path: "conf.py", Classification: ClsConflict, Action: ActConflictCopy}},
		},
	}

	require.NoError(t, backupOverwrittenLocals(e, rb))

	// Each original path is freed (moved aside), and a .LOCAL copy holding the
	// local bytes sits beside it.
	for _, orig := range []string{modified, deleted, conflicted} {
		assert.NoFileExists(t, orig, "the original is moved aside so the remote-side action can proceed")
	}

	require.Len(t, e.localBackups, 3, "one backup per destructive action")

	backups := map[string]string{} // original rel -> backup content

	for _, b := range e.localBackups {
		assert.Contains(t, b, ".LOCAL.", "backup carries the .LOCAL marker")

		content, readErr := os.ReadFile(b)
		require.NoError(t, readErr)

		backups[filepath.Base(b)] = string(content)
	}

	assert.Equal(t, "local edits\n", backups["app.py.LOCAL.20260102T030405Z"])
	assert.Equal(t, "still wanted\n", backups["old.py.LOCAL.20260102T030405Z"])
	assert.Equal(t, "mine\n", backups["conf.py.LOCAL.20260102T030405Z"])
}

// A REMOTE_ADDED path can still hold a file the local manifest never saw (a
// .drignore'd file, a symlink the walk skipped). It gets no .LOCAL copy, so
// applyDownloads must back it up for rollback: a failed sync's Restore then puts
// it back instead of deleting it with nothing behind it (RAPTOR-19348 review).
func TestApplyDownloads_RollbackRestoresAPreexistingRemoteAddedFile(t *testing.T) {
	dir := t.TempDir()

	target := writeLocal(t, dir, "ignored.py", "original\n")

	rb := newTestRollback(t, dir)

	e := &Engine{
		projectDir: dir,
		nowFn:      staticNow,
		files:      &downloadFake{content: map[string]string{"ignored.py": "remote\n"}},
		plan: &SyncPlan{
			Downloads: []FileAction{{Path: "ignored.py", Action: ActDownloadAdd}},
		},
	}

	require.NoError(t, backupOverwrittenLocals(e, rb)) // no-op: REMOTE_ADDED is not in the backup set
	require.NoError(t, applyDownloads(e, rb, codeRefRef{CatalogID: "cat", CatalogVersionID: "ver"}))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "remote\n", string(got), "the download overwrote the pre-existing file")

	// A later phase failing triggers this in production.
	require.NoError(t, rb.Restore())

	got, err = os.ReadFile(target)
	require.NoError(t, err, "the pre-existing file must be restored, not deleted")
	assert.Equal(t, "original\n", string(got))
}

// REMOTE_ADDED has no local file, so there is nothing to back up: the download
// simply creates a new file. A backup here would be an empty artifact.
func TestBackupOverwrittenLocals_SkipsRemoteAddedAndMissingLocals(t *testing.T) {
	dir := t.TempDir()

	rb := newTestRollback(t, dir)

	e := &Engine{
		projectDir: dir,
		nowFn:      staticNow,
		plan: &SyncPlan{
			// REMOTE_ADDED: no local file exists yet.
			Downloads: []FileAction{{Path: "new.py", Action: ActDownloadAdd}},
			// REMOTE_MODIFIED whose local file is (unexpectedly) already gone.
			Deletes: []FileAction{{Path: "vanished.py", Action: ActDownloadDelete}},
		},
	}

	require.NoError(t, backupOverwrittenLocals(e, rb))
	assert.Empty(t, e.localBackups, "nothing local to preserve, so no backups")
}
