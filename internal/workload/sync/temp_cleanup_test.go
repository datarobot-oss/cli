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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file pins the "no litter" contract of a sync run: after a sync
// finishes — successfully or not — every temporary artifact it created must
// be gone. Two real temp producers exist:
//
//   - buildZip creates wapi-sync-*.zip in the SYSTEM temp dir, removed by
//     ApplyUploads' defer on the happy path and by buildZip's error paths.
//   - AtomicWriteFile creates <name>.tmp.* siblings inside the state dir,
//     removed on any failure before the rename.
//
// The rollback tree is deliberately NOT asserted to vanish on failure: a
// failed sync leaves .datarobot/workload/.rollback in place on purpose, as
// the only copy of the user's overwritten files until the next run's Phase 0
// restores-and-removes it (Restore is best-effort, so the tree outlives a
// partial restore). That behaviour is pinned by the interruption tests; here
// it is asserted only where the design says it must hold (gone on success,
// present for recovery on failure), never treated as litter.

// syncTempPrefix is the CreateTemp pattern buildZip uses for its archive.
const syncTempPrefix = "wapi-sync-"

// snapshotSyncTemps lists the sync-pattern entries currently in the system
// temp dir. The before/after diff form matters: os.TempDir is shared with
// every other process on the machine, so "the set did not grow" is the only
// honest cleanup assertion — a pre-existing stray from a crashed old build
// must not make this test fail, and a concurrent creator must not make it
// pass vacuously.
func snapshotSyncTemps(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)

	var found []string

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), syncTempPrefix) {
			found = append(found, e.Name())
		}
	}

	return found
}

// stateDirTmpLitter returns every file under the project's state dir whose
// name matches AtomicWriteFile's temp pattern (<base>.tmp.*). Persistent
// state (config.json, manifest.json, history.log, sync.lock) never matches.
func stateDirTmpLitter(t *testing.T, dir string) []string {
	t.Helper()

	var litter []string

	err := filepath.WalkDir(wapi.Dir(dir), func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if !d.IsDir() && strings.Contains(d.Name(), ".tmp.") {
			litter = append(litter, p)
		}

		return nil
	})
	require.NoError(t, err)

	return litter
}

// assertNoNewSyncTemps diffs the system-temp snapshot against the current
// set and fails when the sync added entries.
func assertNoNewSyncTemps(t *testing.T, before []string) {
	t.Helper()

	after := snapshotSyncTemps(t)

	extra := setMinus(after, before)

	assert.Empty(t, extra, "sync must leave no wapi-sync-* files in the system temp dir")
}

// setMinus returns the elements of a not present in b, sorted.
func setMinus(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}

	var out []string

	for _, s := range a {
		if !inB[s] {
			out = append(out, s)
		}
	}

	return out
}

// newCleanupEngine wires an engine against a fake server for the cleanup
// tests, mirroring the seam the rest of the suite uses. The fake is built by
// the caller because the stage/zip fault hooks chain on the constructor.
func newCleanupEngine(t *testing.T, dir string, files *fakeFilesClient) *Engine {
	t.Helper()

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: files,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e
}

// TestSyncCleanup_StagePathSuccess proves a successful small sync (stage
// path, well under the zip threshold) leaves nothing behind: no zip temp in
// the system temp dir, no AtomicWriteFile litter in the state dir, and no
// rollback tree (Discard removes it once Phase 6 persisted).
func TestSyncCleanup_StagePathSuccess(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":     "print('ok')\n",
		"utils/h.py": "def h(): pass\n",
	})

	files := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	before := snapshotSyncTemps(t)

	e := newCleanupEngine(t, dir, files)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.NotEmpty(t, plan.Uploads)

	_, err = e.Execute(plan)
	require.NoError(t, err)

	assert.Positive(t, files.UploadToStageCalls(), "precondition: the stage path ran")

	assertNoNewSyncTemps(t, before)
	assert.Empty(t, stateDirTmpLitter(t, dir), "no AtomicWriteFile temps may survive a successful sync")
	assert.NoDirExists(t, wapi.RollbackDir(dir), "the rollback tree is discarded once the sync persisted")
}

// TestSyncCleanup_StagePathFailure proves a failed stage sync is equally
// litter-free in both temp locations. The rollback tree remains by design —
// the next run's Phase 0 restores and removes it — so its presence is
// asserted here as the documented exception, not as a pass for litter.
func TestSyncCleanup_StagePathFailure(t *testing.T) {
	dir := initProject(t, map[string]string{
		"a.py": "aaa\n",
		"b.py": "bbb\n",
		"c.py": "ccc\n",
	})

	files := (&fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}).withFailNthUpload(2)

	before := snapshotSyncTemps(t)

	e := newCleanupEngine(t, dir, files)

	plan, err := e.Plan()
	require.NoError(t, err)

	_, err = e.Execute(plan)
	require.Error(t, err, "the injected upload failure must fail the sync")

	assertNoNewSyncTemps(t, before)
	assert.Empty(t, stateDirTmpLitter(t, dir), "Phase 6 never ran, but no AtomicWriteFile temp may survive either")

	// Deliberate design, pinned by the interruption tests: the rollback tree
	// is the recovery copy for a run that died before it could restore. It is
	// consumed (restored + removed) by Phase 0 of the next sync.
	assert.DirExists(t, wapi.RollbackDir(dir),
		"rollback tree must remain after a failed sync for stale-rollback recovery")
}

// zipThresholdProject builds a project whose upload count crosses the
// ChooseUploader zip threshold, so Execute exercises buildZip's temp file.
func zipThresholdProject(t *testing.T) string {
	t.Helper()

	files := make(map[string]string, 21)

	for i := 1; i <= 21; i++ {
		name := fmt.Sprintf("file%02d.txt", i)
		files[name] = fmt.Sprintf("content %d\n", i)
	}

	return initProject(t, files)
}

// TestSyncCleanup_ZipPathSuccess proves a successful zip-path sync removes
// its archive temp from the system temp dir and leaves no other litter.
func TestSyncCleanup_ZipPathSuccess(t *testing.T) {
	dir := zipThresholdProject(t)

	files := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	before := snapshotSyncTemps(t)

	e := newCleanupEngine(t, dir, files)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Greater(t, len(plan.Uploads), 20, "precondition: the plan must cross the zip threshold")

	_, err = e.Execute(plan)
	require.NoError(t, err)

	assert.Positive(t, files.UploadFromZipCalls(), "precondition: the zip path ran")

	assertNoNewSyncTemps(t, before)
	assert.Empty(t, stateDirTmpLitter(t, dir))
	assert.NoDirExists(t, wapi.RollbackDir(dir))
}

// TestSyncCleanup_ZipPathFailure proves the failure path of buildZip also
// cleans its temp: the plan is built, then one upload's source file is
// deleted, so addToZip fails on an open error while the archive temp
// already exists on disk. buildZip must remove it before propagating.
func TestSyncCleanup_ZipPathFailure(t *testing.T) {
	dir := zipThresholdProject(t)

	files := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	before := snapshotSyncTemps(t)

	e := newCleanupEngine(t, dir, files)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Remove one file that the plan uploads, after planning: the zip build
	// then fails mid-archive with the temp file already on disk.
	require.NoError(t, os.Remove(filepath.Join(dir, "file01.txt")))

	_, err = e.Execute(plan)
	require.Error(t, err, "the deleted source must fail the zip build")

	// Pin the failure to buildZip's per-file open ("... for zip: ..."), the
	// only error text in the zip path carrying that phrase: if Execute ever
	// failed earlier (rollback journal, a delete, an uploader swap), the
	// litter assertions below would otherwise pass vacuously on a run that
	// never reached the archive build.
	require.ErrorContains(t, err, "for zip")

	assertNoNewSyncTemps(t, before)
	assert.Empty(t, stateDirTmpLitter(t, dir))
}
