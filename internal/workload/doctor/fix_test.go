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

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actionByID indexes a repair run's actions by their check id.
func actionByID(t *testing.T, actions []core.Action) map[string]core.Action {
	t.Helper()

	byID := make(map[string]core.Action, len(actions))

	for _, a := range actions {
		byID[a.ID] = a
	}

	return byID
}

// requireActionsInOrder asserts the pinned action order: manifest rebuild,
// rollback clear, lock clear.
func requireActionsInOrder(t *testing.T, actions []core.Action) {
	t.Helper()

	require.Len(t, actions, 3)

	require.Equal(t, CheckIDManifest, actions[0].ID)
	require.Equal(t, CheckIDRollback, actions[1].ID)
	require.Equal(t, CheckIDLock, actions[2].ID)
}

// TestRunFix_HealthyProject_AllNotNeeded verifies that on a healthy project
// every repair reports not-needed and the filesystem is left untouched (in
// particular, no sync.lock is created).
func TestRunFix_HealthyProject_AllNotNeeded(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))
	require.NoError(t, wapi.SaveManifest(dir, validManifest("")))

	before := stateFileHashes(t, dir)

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	for _, a := range actions {
		assert.Equal(t, core.ActionNotNeeded, a.Status, "action %s", a.ID)
		assert.Empty(t, a.Reason, "action %s", a.ID)
	}

	assert.Equal(t, before, stateFileHashes(t, dir), "a no-op fix must not write anything")

	_, err := os.Stat(lockPath(t, dir))

	assert.ErrorIs(t, err, os.ErrNotExist, "fix must not create sync.lock")
}

// TestRunFix_MissingManifest_RebuiltEmptyBase verifies the rebuilt manifest is
// an empty BASE derived from config, honoring both-or-neither.
func TestRunFix_MissingManifest_RebuiltEmptyBase(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	rebuild := actionByID(t, actions)[CheckIDManifest]

	assert.Equal(t, core.ActionPerformed, rebuild.Status)

	m, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	assert.Equal(t, wapi.ManifestVersion, m.Version)
	assert.Nil(t, m.SyncedVersionID, "config has no lastSyncedVersionId, so the rebuilt pointer must be nil")
	assert.Nil(t, m.SyncedAt, "syncedAt must stay nil when syncedVersionId is nil (both-or-neither)")
	assert.Empty(t, m.Files, "rebuild resets to an empty BASE")
}

// TestRunFix_CorruptManifest_Rebuilt verifies truncated JSON is replaced by a
// valid empty BASE.
func TestRunFix_CorruptManifest_Rebuilt(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	writeStateFile(t, dir, "manifest.json", `{"version":1,"syncedAt":nul`)

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDManifest].Status)

	_, err := wapi.LoadManifest(dir)

	require.NoError(t, err, "manifest must parse after the rebuild")
}

// TestRunFix_DivergentManifest_ConfigWins verifies a valid but divergent
// manifest is reset from config, with both-or-neither honored on the rebuilt
// synced pointers.
func TestRunFix_DivergentManifest_ConfigWins(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	// Manifest claims a different synced version than config.
	require.NoError(t, wapi.SaveManifest(dir, validManifest("65f1a2b3c4d5e6f7a8b9c0ff")))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDManifest].Status)

	m, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	require.NotNil(t, m.SyncedVersionID)

	assert.Equal(t, testVersionID, *m.SyncedVersionID, "config wins the divergence")
	require.NotNil(t, m.SyncedAt, "syncedAt must be non-nil iff syncedVersionId is non-nil")
}

// TestRunFix_CorruptConfig_ManifestSkippedWithReinitRemedy verifies that
// without a valid config the manifest cannot be rebuilt and the skip reason
// points at re-initialization.
func TestRunFix_CorruptConfig_ManifestSkippedWithReinitRemedy(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	writeStateFile(t, dir, "config.json", `{"artifactId":"abc`)

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	byID := actionByID(t, actions)

	rebuild := byID[CheckIDManifest]

	assert.Equal(t, core.ActionSkipped, rebuild.Status)
	assert.Contains(t, rebuild.Reason, "config")
	assert.Contains(t, rebuild.Reason, "init", "skip reason must carry the re-init remedy")

	// The other repairs still attempt independently of the config state.
	assert.Equal(t, core.ActionNotNeeded, byID[CheckIDRollback].Status)
	assert.Equal(t, core.ActionNotNeeded, byID[CheckIDLock].Status)

	_, statErr := os.Stat(filepath.Join(wapi.Dir(dir), "manifest.json"))

	assert.ErrorIs(t, statErr, os.ErrNotExist, "no manifest may be written without a valid config")
}

// TestRunFix_UnlinkedProject_SkipsManifestRestNotNeeded pins the unlinked
// behavior: no linked state means nothing to rebuild from and nothing to fix.
func TestRunFix_UnlinkedProject_SkipsManifestRestNotNeeded(t *testing.T) {
	dir := t.TempDir()

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	byID := actionByID(t, actions)

	rebuild := byID[CheckIDManifest]

	assert.Equal(t, core.ActionSkipped, rebuild.Status)
	assert.Contains(t, rebuild.Reason, "no linked state")

	assert.Equal(t, core.ActionNotNeeded, byID[CheckIDRollback].Status)
	assert.Equal(t, core.ActionNotNeeded, byID[CheckIDLock].Status)
}

// seedRollback writes a rollback tree with one backed-up file.
func seedRollback(t *testing.T, projectDir, relPath, contents string) {
	t.Helper()

	dst := filepath.Join(wapi.Dir(projectDir), ".rollback", filepath.FromSlash(relPath))

	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))

	require.NoError(t, os.WriteFile(dst, []byte(contents), 0o600))
}

// TestRunFix_RollbackRestoredAndRemoved verifies backed-up files return to
// their original paths and the .rollback/ tree is removed.
func TestRunFix_RollbackRestoredAndRemoved(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))
	require.NoError(t, wapi.SaveManifest(dir, validManifest("")))

	seedRollback(t, dir, "app/main.go", "backed up contents")

	// The working-tree copy drifted after the backup was staged.
	working := filepath.Join(dir, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("drifted contents"), 0o600))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDRollback].Status)

	restored, err := os.ReadFile(working)

	require.NoError(t, err)

	assert.Equal(t, "backed up contents", string(restored), "the backed-up file must return to its original path")

	_, statErr := os.Stat(filepath.Join(wapi.Dir(dir), ".rollback"))

	assert.ErrorIs(t, statErr, os.ErrNotExist, ".rollback/ must be removed after the restore")
}

// TestRunFix_RollbackRecreatesDeletedFile verifies a file the interrupted sync
// had deleted comes back from the backup tree.
func TestRunFix_RollbackRecreatesDeletedFile(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	seedRollback(t, dir, "app/removed.go", "resurrected")

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDRollback].Status)

	restored, err := os.ReadFile(filepath.Join(dir, "app", "removed.go"))

	require.NoError(t, err)

	assert.Equal(t, "resurrected", string(restored))
}

// TestRunFix_EmptyRollbackDirHandled pins the empty-tree case: a bare
// .rollback/ directory still counts as an interrupted rollback and --fix
// clears it.
func TestRunFix_EmptyRollbackDirHandled(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	require.NoError(t, os.Mkdir(filepath.Join(wapi.Dir(dir), ".rollback"), 0o755))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDRollback].Status)

	_, statErr := os.Stat(filepath.Join(wapi.Dir(dir), ".rollback"))

	assert.ErrorIs(t, statErr, os.ErrNotExist, "an empty .rollback/ must still be removed")
}

// TestRunFix_RollbackAbsent_NotNeeded pins that a healthy rollback state
// reports not-needed and writes nothing.
func TestRunFix_RollbackAbsent_NotNeeded(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionNotNeeded, actionByID(t, actions)[CheckIDRollback].Status)
}

// TestRunFix_LockAbsent_NotNeededAndNotCreated verifies that with no sync.lock
// file the repair reports not-needed and must NOT create the file.
func TestRunFix_LockAbsent_NotNeededAndNotCreated(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionNotNeeded, actionByID(t, actions)[CheckIDLock].Status)

	_, err := os.Stat(lockPath(t, dir))

	assert.ErrorIs(t, err, os.ErrNotExist, "the lock repair must not create sync.lock")
}

// TestRunFix_LockAcquirable_VerifiedNotNeeded verifies a stale but unheld lock
// file is verified acquirable (acquired and released) and reported
// not-needed — the file itself is never removed and the lock stays acquirable
// afterwards.
func TestRunFix_LockAcquirable_VerifiedNotNeeded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows path is covered by the seam tests")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	actions := RunFix(context.Background(), dir)

	lockAction := actionByID(t, actions)[CheckIDLock]

	assert.Equal(t, core.ActionNotNeeded, lockAction.Status)

	// Pin the probe-path reason string: the lock was verified acquirable
	// (acquired and released) with no holder detected.
	assert.Contains(t, lockAction.Reason, "verified acquirable",
		"the acquirable-lock probe path must carry the 'verified acquirable' reason")

	// After the verify, the lock check must report OK (acquirable), and the
	// probe must still be able to acquire and release within this process.
	res := (&lockCheck{projectDir: dir, goos: runtime.GOOS}).Run(context.Background())

	assert.Equal(t, core.StatusOK, res.Status)
}

// holdLockForTest holds sync.lock from a second open file description in the
// test process, exactly like a live second CLI process would. The returned
// release function must be called to clean up.
func holdLockForTest(t *testing.T, projectDir string) func() {
	t.Helper()

	f, err := os.OpenFile(lockPath(t, projectDir), os.O_RDWR, 0o600)

	require.NoError(t, err)

	require.NoError(t, tryLockSyncLockExclusive(f))

	return func() {
		_ = unlockSyncLock(f)
		_ = f.Close()
	}
}

// TestRunFix_LockHeld_SkipsAllRepairsStateUntouched verifies that when a live
// process holds the lock, EVERY repair is skipped with the sync-in-progress
// reason and no state is touched.
func TestRunFix_LockHeld_SkipsAllRepairsStateUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows path is covered by the seam tests")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	// A repairable problem (missing manifest) that must NOT be repaired.
	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	before := stateFileHashes(t, dir)

	release := holdLockForTest(t, dir)

	defer release()

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	for _, a := range actions {
		require.Equal(t, core.ActionSkipped, a.Status, "action %s", a.ID)
		assert.Contains(t, a.Reason, "sync in progress", "action %s", a.ID)
	}

	assert.Equal(t, before, stateFileHashes(t, dir), "a gated fix run must not write anything")

	_, statErr := os.Stat(filepath.Join(wapi.Dir(dir), "manifest.json"))

	assert.ErrorIs(t, statErr, os.ErrNotExist, "the manifest must NOT be rebuilt under a held lock")
}

// TestRunFix_UninspectableLock_SkipsAllRepairs pins the conservative gate: a
// lock that cannot be inspected might hide a live sync, so no repair runs.
func TestRunFix_UninspectableLock_SkipsAllRepairs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics; windows reports SKIP via the seam")
	}

	if os.Geteuid() == 0 {
		t.Skip("root can open unreadable files, so the uninspectable state cannot be fabricated")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	require.NoError(t, os.Chmod(lockPath(t, dir), 0o000))

	t.Cleanup(func() {
		if chmodErr := os.Chmod(lockPath(t, dir), 0o600); chmodErr != nil {
			t.Logf("restore lock file permissions: %v", chmodErr)
		}
	})

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	for _, a := range actions {
		require.Equal(t, core.ActionSkipped, a.Status, "action %s", a.ID)
		assert.Contains(t, a.Reason, "cannot inspect", "action %s", a.ID)
	}
}

// TestRunFix_PartialFailure_OthersStillPerformed verifies a repair that fails
// mid-write is reported skipped with the error as reason while the remaining
// repairs still run.
func TestRunFix_PartialFailure_OthersStillPerformed(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	// Force the manifest rebuild to fail: manifest.json exists as a
	// DIRECTORY, so reading it corrupts and atomically writing over it fails.
	require.NoError(t, os.Mkdir(filepath.Join(wapi.Dir(dir), "manifest.json"), 0o755))

	seedRollback(t, dir, "app/main.go", "backed up contents")

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	byID := actionByID(t, actions)

	rebuild := byID[CheckIDManifest]

	assert.Equal(t, core.ActionSkipped, rebuild.Status)
	assert.NotEmpty(t, rebuild.Reason, "the failed repair must carry the error as its reason")

	assert.Equal(t, core.ActionPerformed, byID[CheckIDRollback].Status, "the rollback repair must still attempt")

	restored, readErr := os.ReadFile(filepath.Join(dir, "app", "main.go"))

	require.NoError(t, readErr)

	assert.Equal(t, "backed up contents", string(restored))
}

// TestRunFix_ManifestRebuild_WorkingTreeUntouched verifies the manifest
// rebuild only rewrites the state file; every working-tree file keeps its
// exact checksum.
func TestRunFix_ManifestRebuild_WorkingTreeUntouched(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	writeStateFile(t, dir, "manifest.json", `{"version":1,"files":{}}`)

	working := filepath.Join(dir, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("user source"), 0o600))

	before := stateFileHashes(t, dir)

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionPerformed, actionByID(t, actions)[CheckIDManifest].Status)

	after := stateFileHashes(t, dir)

	// Only manifest.json itself may change; every other path — the whole
	// working tree — must be byte-identical.
	for path, want := range before {
		if filepath.Base(path) == "manifest.json" {
			continue
		}

		got, ok := after[path]

		require.True(t, ok, "file disappeared: %s", path)
		assert.Equal(t, want, got, "file must be untouched by the rebuild: %s", path)
	}
}

// TestRunFix_LockFileNeverRemoved pins Release semantics at the repair level:
// verifying an acquirable lock never unlinks the file (a waiter could hold
// the open descriptor).
func TestRunFix_LockFileNeverRemoved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows path is covered by the seam tests")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	actions := RunFix(context.Background(), dir)

	assert.Equal(t, core.ActionNotNeeded, actionByID(t, actions)[CheckIDLock].Status)

	_, err := os.Stat(lockPath(t, dir))

	assert.NoError(t, err, "the lock file itself must never be removed")
}

// TestRunFix_WindowsGate_Proceeds pins the windows gate behavior: the lock
// probe SKIPs there (flock not enforced), and repairs still run — consistent
// with sync itself not being exclusive on Windows (RAPTOR-16928).
func TestRunFix_WindowsGate_Proceeds(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	// Run the repair suite the way a windows host would see it: the gate
	// probe SKIPs (exercised through the injected platform seam) and the
	// repairs still proceed.
	actions := runFixWithGoos(context.Background(), dir, "windows")

	requireActionsInOrder(t, actions)

	assert.Equal(t, core.ActionNotNeeded, actionByID(t, actions)[CheckIDLock].Status)
}

// TestRunFix_MultipleProblems_AllRepairedInOneRun verifies that with
// simultaneously corrupt manifest, stale .rollback/ (with a modified project
// file), and a dead sync.lock, each repair gets its own action entry; the
// post-fix state is healthy; non-rollback working-tree files are unchanged.
func TestRunFix_MultipleProblems_AllRepairedInOneRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows gate path is covered by the seam tests")
	}

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	// Problem 1: corrupt manifest (truncated JSON).
	writeStateFile(t, dir, "manifest.json", `{"version":1,"syncedAt":nul`)

	// Problem 2: stale .rollback/ with a backed-up file that overwrites a
	// modified working-tree copy.
	seedRollback(t, dir, "app/main.go", "backed up contents")

	working := filepath.Join(dir, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("drifted contents"), 0o600))

	// Problem 3: a dead (unheld) sync.lock file.
	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	// A non-state working-tree file that must survive untouched.
	extra := filepath.Join(dir, "lib", "util.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(extra), 0o755))

	require.NoError(t, os.WriteFile(extra, []byte("package lib"), 0o600))

	beforeExtra, err := os.ReadFile(extra)

	require.NoError(t, err)

	actions := RunFix(context.Background(), dir)

	requireActionsInOrder(t, actions)

	byID := actionByID(t, actions)

	// Each repair gets its own action entry with a distinct status.
	assert.Equal(t, core.ActionPerformed, byID[CheckIDManifest].Status, "manifest rebuild performed")
	assert.Equal(t, core.ActionPerformed, byID[CheckIDRollback].Status, "rollback restore performed")
	assert.Equal(t, core.ActionNotNeeded, byID[CheckIDLock].Status, "lock clear not-needed (acquirable)")

	// Post-fix: manifest parses and is an empty BASE.
	m, loadErr := wapi.LoadManifest(dir)

	require.NoError(t, loadErr)

	assert.Empty(t, m.Files)
	assert.Nil(t, m.SyncedVersionID)
	assert.Nil(t, m.SyncedAt)

	// Post-fix: .rollback/ removed and working-tree file restored.
	restored, readErr := os.ReadFile(working)

	require.NoError(t, readErr)

	assert.Equal(t, "backed up contents", string(restored))

	_, statErr := os.Stat(filepath.Join(wapi.Dir(dir), ".rollback"))

	require.ErrorIs(t, statErr, os.ErrNotExist, ".rollback/ must be removed")

	// Non-rollback working-tree files are unchanged.
	extraAfter, err := os.ReadFile(extra)

	require.NoError(t, err)

	assert.Equal(t, string(beforeExtra), string(extraAfter), "non-rollback files must be untouched")
}
