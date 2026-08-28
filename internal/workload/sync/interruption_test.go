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
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInterruption_MidUpload_NonZeroAndConverges simulates a sync interrupted
// mid-upload and verifies that (1) the interrupted sync exits non-zero without
// advancing manifest.json or config.json, (2) the rollback directory remains
// for stale-rollback recovery, and (3) a following sync converges to a manifest
// matching the fake's server state.
//
// The interruption is simulated by injecting an upload failure (withFailNthUpload)
// rather than by delivering a real SIGINT. The effect is the same from the
// engine's perspective: Phase 5 fails, Phase 6 never runs, and the rollback
// directory is left on disk. The second sync's Phase 0 restores the stale
// rollback via RestoreStaleIfPresent, then proceeds normally.
//
// The fake for the second sync is pre-populated with the prior version (the
// server state from the original successful sync) so that ApplyStage's REPLACE
// merge produces a version containing both the uploaded files and the
// unchanged files (like .drignore). Without this, the fake's server would
// only hold the uploaded files and the convergence check would fail for
// unchanged paths that the manifest correctly carries forward from BASE.
//
// Go-level coverage:
//   - The first sync returns an error (non-zero) without advancing manifest
//     or config.
//   - The rollback directory exists after the failed sync (it is NOT cleaned
//     up by Restore, only by Discard on success or by the next sync's Phase 0).
//   - The second sync's Phase 0 restores the stale rollback
//     (e.StaleRollbackRestored() == true) and removes the rollback directory.
//   - The second sync converges: the manifest's hashes match the fake's
//     AllFiles server state for every file.
//
// What remains for a staging validator to confirm through the real binary:
//   - Real SIGINT delivery to a running `dr` process (a Go test cannot
//     deliver a signal to itself in a meaningful way; the test simulates the
//     interruption via a fault-injected upload failure, which has the same
//     engine-level effect: Phase 5 fails, Phase 6 never runs, the rollback
//     directory remains for the next sync's stale-rollback recovery).
//   - The real binary exits with a non-zero code on SIGINT (the Go test
//     asserts the engine returns an error, which maps to a non-zero exit in
//     the CLI, but the signal-to-exit-code path through the CLI's signal
//     handler is the CLI's responsibility, not the engine's).
//
// Fulfills VAL-CROSS-011.
func TestInterruption_MidUpload_NonZeroAndConverges(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	files := map[string]string{
		"a.py": "aaa\n",
		"b.py": "bbb\n",
		"c.py": "ccc\n",
	}

	mods := map[string]string{
		"a.py": "AAAA\n",
		"b.py": "BBBB\n",
		"c.py": "CCCC\n",
	}

	dir := syncedProject(t, files, catalogID, versionID)

	// Compute the prior version's server state (old content hashes) before
	// modifying any files. The second sync's fake must be pre-populated with
	// this so ApplyStage's REPLACE merge yields a version containing both
	// the uploaded files and the unchanged ones (like .drignore).
	priorVersion := make(map[string]filesapi.FileMeta, len(files)+1)

	for rel := range files {
		hash, size, err := hashLocal(t, dir, rel)
		require.NoError(t, err)

		priorVersion[rel] = filesapi.FileMeta{Hash: hash, Size: size}
	}

	// Include .drignore (created by Initialize) so it is in the server state.
	drHash, drSize, err := hashLocal(t, dir, ignore.FileName)
	require.NoError(t, err)

	priorVersion[ignore.FileName] = filesapi.FileMeta{Hash: drHash, Size: drSize}

	// Now modify the files to introduce pending changes.
	for rel, content := range mods {
		modifyFile(t, dir, rel, content)
	}

	// Capture pre-sync state.
	pre := captureState(t, dir, []string{"a.py", "b.py", "c.py"})

	// --- First sync: simulate interruption mid-upload ---

	fake1 := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-new",
	}).withFailNthUpload(2)

	e1, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake1,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	plan1, err := e1.Plan()
	require.NoError(t, err)

	require.Len(t, plan1.Uploads, 3, "all three files should be pending uploads")

	_, err = e1.Execute(plan1)

	require.Error(t, err, "interrupted sync must exit non-zero (error)")
	assert.Contains(t, err.Error(), "upload",
		"error must come from the upload step")

	require.NoError(t, e1.Close())

	// manifest.json and config.json must not be advanced.
	assertStateUntouched(t, dir, pre)

	// The rollback directory must exist — it is the stale rollback that
	// the next sync's Phase 0 will restore.
	rollDir := wapi.RollbackDir(dir)

	_, err = os.Stat(rollDir)
	require.NoError(t, err,
		"rollback directory must exist after a failed sync (for stale-rollback recovery)")

	// ApplyStage must not have been called — the upload failure prevented it.
	assert.Equal(t, 0, fake1.ApplyStageCalls(),
		"ApplyStage must not be called when the upload was interrupted")

	// --- Second sync: converge ---

	// Pre-populate the fake with the prior version so ApplyStage's REPLACE
	// merge produces a version containing both uploaded and unchanged files.
	fake2 := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-2",
		versionID: "ver-new",
	}).withVersion(catalogID, versionID, priorVersion)

	e2, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake2,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e2.Close() })

	// Phase 0 must restore the stale rollback from the first sync.
	plan2, err := e2.Plan()
	require.NoError(t, err)

	assert.True(t, e2.StaleRollbackRestored(),
		"Phase 0 must restore the stale rollback from the interrupted sync")

	require.Len(t, plan2.Uploads, 3,
		"the same pending changes must still produce uploads")

	// Execute the second sync — it must converge.
	result, err := e2.Execute(plan2)
	require.NoError(t, err, "the second sync must succeed and converge")

	require.NotNil(t, result)

	// The rollback directory must be gone after a successful sync.
	_, err = os.Stat(rollDir)
	assert.True(t, os.IsNotExist(err),
		"rollback directory must be removed after a successful sync")

	// The manifest must match the fake's server state for every file.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	all, err := fake2.AllFiles(catalogID, "ver-new")
	require.NoError(t, err)

	// Set-size equality closes the reverse direction: the loop below proves
	// every manifest path exists on the server, but without this check a
	// path the server holds and the manifest forgot would fall out of the
	// record silently. Map keys are unique, so equal sizes plus the
	// per-path matches below mean the two sets are exactly equal.
	assert.Len(t, all, len(manifest.Files),
		"manifest and server must hold exactly the same path set after convergence")

	// Every file in the manifest must have a hash matching the server.
	for path, fm := range manifest.Files {
		serverFM, ok := all[path]
		require.True(t, ok, "server must have %s", path)

		assert.Equal(t, fm.Hash, serverFM.Hash,
			"manifest hash for %s must match the server's checksum (converged)", path)
		assert.Equal(t, fm.Size, serverFM.Size,
			"manifest size for %s must match the server's size", path)
	}

	// The manifest's syncedVersionId must equal the new version.
	require.NotNil(t, manifest.SyncedVersionID)
	assert.Equal(t, "ver-new", *manifest.SyncedVersionID,
		"manifest syncedVersionId must equal the new version after convergence")

	// config.json's LastSyncedVersionID must also equal the new version.
	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, "ver-new", *cfg.LastSyncedVersionID,
		"config LastSyncedVersionID must equal the new version after convergence")
}
