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

// Defense-in-depth: even if filesapi.AllFiles validates manifest entries,
// FileAction.Path values flow through SyncPlan before reaching the
// filesystem, so the sync engine must re-validate at the write/delete
// site. These tests pin the rejection contract for CFX-6228 / CFX-6229.

// unsafeServerPaths are the bypass classes every server-controlled-path
// guard in this package must reject. Shared across the two call-site
// tests so a new bypass class added here is covered at both sites.
var unsafeServerPaths = []string{
	"../escape",
	"../../etc/passwd",
	"/etc/passwd",
	`..\windows`,
	"",
}

func TestDownloadOne_RejectsUnsafeServerPath(t *testing.T) {
	for _, bad := range unsafeServerPaths {
		t.Run(bad, func(t *testing.T) {
			// fakeFilesClient.DownloadFile returns "DownloadFile not
			// expected" — if SafeRelPath fails first the error message
			// is the unsafe-path wrapper, proving no remote call ran.
			e := &Engine{projectDir: t.TempDir(), files: &fakeFilesClient{}}

			err := downloadOne(e, "cid", "vid", FileAction{Path: bad})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned unsafe download path")
			assert.NotContains(t, err.Error(), "DownloadFile not expected")
		})
	}
}

func TestRemoveLocalDeletedFiles_RejectsUnsafeServerPath(t *testing.T) {
	for _, bad := range unsafeServerPaths {
		t.Run(bad, func(t *testing.T) {
			e := &Engine{
				projectDir: t.TempDir(),
				plan: &SyncPlan{
					Deletes: []FileAction{{Path: bad, Action: ActDownloadDelete}},
				},
			}

			err := removeLocalDeletedFiles(e)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned unsafe delete path")
		})
	}
}

func TestRemoveLocalDeletedFiles_DoesNotDeleteOutsideProjectDir(t *testing.T) {
	dir := t.TempDir()

	// Sentinel sits in the temp dir's parent; ../sentinel would resolve
	// to it under a naive filepath.Join, so its survival proves the
	// SafeRelPath guard short-circuited before os.Remove.
	sentinel := filepath.Join(filepath.Dir(dir), "sentinel-"+filepath.Base(dir))
	require.NoError(t, os.WriteFile(sentinel, []byte("keep me"), 0o644))

	t.Cleanup(func() { _ = os.Remove(sentinel) })

	e := &Engine{
		projectDir: dir,
		plan: &SyncPlan{
			Deletes: []FileAction{
				{Path: "../" + filepath.Base(sentinel), Action: ActDownloadDelete},
			},
		},
	}

	err := removeLocalDeletedFiles(e)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server returned unsafe delete path")

	_, statErr := os.Stat(sentinel)
	require.NoError(t, statErr, "sentinel outside project dir must not have been removed")
}

// Non-ActDownloadDelete entries must be skipped without validation —
// uploads carry locally-scanned paths that don't need this guard, and
// running SafeRelPath on them would over-reject.
func TestRemoveLocalDeletedFiles_IgnoresNonDownloadDeleteEntries(t *testing.T) {
	e := &Engine{
		projectDir: t.TempDir(),
		plan: &SyncPlan{
			Deletes: []FileAction{
				{Path: "../would-be-unsafe", Action: ActUploadDelete},
			},
		},
	}

	require.NoError(t, removeLocalDeletedFiles(e))
}

// validateServerPaths is the up-front phase5 guard that runs before any
// filesystem op (including Rollback.Backup). The per-call-site guards in
// downloadOne / removeLocalDeletedFiles back it up, but this guard is the
// one that keeps unsafe paths out of rb.Backup's stat/open/copy calls.

func TestValidateServerPaths_RejectsUnsafeDownload(t *testing.T) {
	for _, bad := range unsafeServerPaths {
		t.Run(bad, func(t *testing.T) {
			plan := &SyncPlan{Downloads: []FileAction{{Path: bad}}}

			err := validateServerPaths(plan)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned unsafe download path")
		})
	}
}

func TestValidateServerPaths_RejectsUnsafeConflict(t *testing.T) {
	for _, bad := range unsafeServerPaths {
		t.Run(bad, func(t *testing.T) {
			plan := &SyncPlan{Conflicts: []FileAction{{Path: bad}}}

			err := validateServerPaths(plan)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned unsafe conflict path")
		})
	}
}

func TestValidateServerPaths_RejectsUnsafeDownloadDelete(t *testing.T) {
	for _, bad := range unsafeServerPaths {
		t.Run(bad, func(t *testing.T) {
			plan := &SyncPlan{
				Deletes: []FileAction{{Path: bad, Action: ActDownloadDelete}},
			}

			err := validateServerPaths(plan)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned unsafe delete path")
		})
	}
}

// Uploads and ActUploadDelete entries carry locally-scanned paths that
// the local walker has already validated; re-checking them here would
// over-reject benign inputs.
func TestValidateServerPaths_SkipsLocalDrivenEntries(t *testing.T) {
	plan := &SyncPlan{
		Uploads: []FileAction{{Path: "../would-be-unsafe"}},
		Deletes: []FileAction{{Path: "../would-be-unsafe", Action: ActUploadDelete}},
	}

	require.NoError(t, validateServerPaths(plan))
}

// Phase-level regression: an unsafe path in plan.Downloads would
// otherwise reach rb.Backup via backupDownloadTargets before any
// per-call-site guard fired. The up-front check must short-circuit
// before NewRollback runs, so no .wapi/.rollback dir is created and
// no bytes outside e.projectDir are read.
func TestPhase5Execute_RejectsUnsafePathBeforeAnyFilesystemOp(t *testing.T) {
	dir := t.TempDir()

	sentinel := filepath.Join(filepath.Dir(dir), "sentinel-"+filepath.Base(dir))
	require.NoError(t, os.WriteFile(sentinel, []byte("keep me"), 0o644))

	t.Cleanup(func() { _ = os.Remove(sentinel) })

	e := &Engine{
		projectDir: dir,
		plan: &SyncPlan{
			Downloads: []FileAction{
				{Path: "../" + filepath.Base(sentinel), Action: ActDownloadAdd},
			},
		},
	}

	err := phase5Execute(e)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server returned unsafe download path")

	_, statErr := os.Stat(filepath.Join(dir, ".wapi", rollbackDirName))
	assert.True(t, os.IsNotExist(statErr), "rollback dir must not be created when validation fails")

	_, statErr = os.Stat(sentinel)
	require.NoError(t, statErr, "sentinel outside project dir must be untouched")
}
