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
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file pin the Phase 6 discard-first ordering: the rollback
// directory must be discarded at Phase 6 ENTRY, before any state write, not
// after the last one succeeds.
//
// The hazard this guards: e.rollback is assigned only after Phase 5 executed
// the whole plan successfully, but a Phase 6 failure (SaveManifest or
// SaveConfig) returns early. If the discard sits below the state writes, a
// write failure strands the rollback dir. The next run's Phase 0
// (RestoreStaleIfPresent) then blindly copies the pre-sync bytes back into
// the working tree, the advanced manifest makes the diff classify them as
// local edits, and the sync re-uploads the stale bytes over the remote —
// a self-perpetuating corruption chain with no prompt and no error.

// rollbackDiscardScenario builds a synced project whose next sync carries one
// upload (a.py, locally modified) and one download (b.py, remotely modified).
// The download is the point: downloads are backed up into the rollback tree,
// so the rollback covers b.py's pre-sync bytes and a stranded rollback dir
// has real bytes to resurrect. An uploads-only plan would strand an empty
// rollback dir and could not demonstrate the corruption.
//
// Server timeline: ver-synced (what the project was synced at) → ver-remote
// (b.py changed remotely, drift) → ver-new (created by this sync's apply,
// which uploads the local a.py on top of ver-remote).
type rollbackDiscardScenario struct {
	dir      string
	fake     *fakeFilesClient
	engine   *Engine
	plan     *SyncPlan
	origA    string // a.py content at sync time
	newA     string // a.py content after the local edit
	origB    string // b.py content at sync time
	remoteB  string // b.py content after the remote edit
	rollDir  string
	cfgBytes []byte // config.json bytes captured before the run
}

// setupRollbackDiscardScenario wires the scenario and returns it after Plan(),
// so the caller can inject a Phase 6 fault and then Execute.
func setupRollbackDiscardScenario(t *testing.T) *rollbackDiscardScenario {
	return setupRollbackDiscardScenarioWithPatchHook(t, nil)
}

// setupRollbackDiscardScenarioWithPatchHook is the hook-aware variant. The
// hook, when non-nil, replaces the no-op PatchCodeRef fake: it runs at the
// very end of Phase 5 — after every backup, download, delete, and upload,
// with the rollback dir already on disk but Phase 6 not yet entered — which
// is exactly the window a Phase 6-entry fault must land in.
func setupRollbackDiscardScenarioWithPatchHook(t *testing.T, patchHook func(artifactID, catalogID, catalogVersionID string) error) *rollbackDiscardScenario {
	t.Helper()

	const (
		catalogID  = "cid-synced"
		oldVersion = "ver-synced"
		remoteVer  = "ver-remote"
		newVersion = "ver-new"
	)

	s := &rollbackDiscardScenario{
		origA:   "aaa\n",
		newA:    "AAAA\n",
		origB:   "bbb\n",
		remoteB: "BBBB-remote\n",
	}

	s.dir = syncedProject(t, map[string]string{
		"a.py": s.origA,
		"b.py": s.origB,
	}, catalogID, oldVersion)

	// Pending local change: a.py becomes an upload.
	modifyFile(t, s.dir, "a.py", s.newA)

	// The remote moved ahead: ver-remote holds the edited b.py. ApplyStage
	// merges the staged a.py into this version, producing ver-new.
	drHash, drSize, err := hashLocal(t, s.dir, ignore.FileName)
	require.NoError(t, err)

	s.fake = (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: newVersion,
	}).withVersion(catalogID, remoteVer, map[string]filesapi.FileMeta{
		"a.py":          {Hash: sha256Hex([]byte(s.origA)), Size: int64(len(s.origA))},
		"b.py":          {Hash: sha256Hex([]byte(s.remoteB)), Size: int64(len(s.remoteB))},
		ignore.FileName: {Hash: drHash, Size: drSize},
	}).withDownloadable(remoteVer, map[string][]byte{
		"a.py": []byte(s.origA),
		"b.py": []byte(s.remoteB),
	})

	// The artifact's codeRef still points at ver-remote, so Phase 1 detects
	// drift against the stale config and fetches real remote state.
	e, err := newWithDeps(s.dir, Options{Yes: true}, Deps{
		Files: s.fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, remoteVer), nil
			},
			// nil PatchFn is safe: the fake no-ops when the hook is unset.
			PatchFn: patchHook,
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	s.engine = e
	s.rollDir = wapi.RollbackDir(s.dir)

	cfg, err := os.ReadFile(wapi.ConfigPath(s.dir))
	require.NoError(t, err)

	s.cfgBytes = cfg

	s.plan, err = e.Plan()
	require.NoError(t, err)

	require.Len(t, s.plan.Uploads, 1, "a.py must be the one upload")
	require.Equal(t, "a.py", s.plan.Uploads[0].Path)
	require.Len(t, s.plan.Downloads, 1, "b.py must be the one download")
	require.Equal(t, "b.py", s.plan.Downloads[0].Path)

	return s
}

// replaceFileWithDir removes a state file and recreates it as a directory, so
// AtomicWriteFile's rename fails when the engine next writes it. Phase 1 has
// already loaded the real file, so the fault hits only Phase 6.
func replaceFileWithDir(t *testing.T, path string) {
	t.Helper()

	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(path) })
}

// assertNoRollbackDir asserts the rollback location is empty — the property
// a Phase 6 outcome must never violate. Non-fatal so a red run reports the
// full downstream corruption (stale restore, false uploads) alongside it.
func assertNoRollbackDir(t *testing.T, rollDir string) {
	t.Helper()

	_, err := os.Stat(rollDir)
	assert.True(t, os.IsNotExist(err),
		"rollback directory must not survive the Phase 6 outcome at %s", rollDir)
}

// TestPhase6SaveConfigFailure_DiscardsRollback_NextRunNoFalseUploads is the
// main regression: SaveConfig fails AFTER SaveManifest advanced the manifest.
// The rollback dir must already be gone (discarded at Phase 6 entry), and the
// next run must reconcile from the remote without resurrecting b.py's
// pre-sync bytes as a false LOCAL_MODIFIED upload.
//
// Fulfills VAL-ROLLBACK-001 (SaveConfig-failure leg) and VAL-ROLLBACK-002.
func TestPhase6SaveConfigFailure_DiscardsRollback_NextRunNoFalseUploads(t *testing.T) {
	const (
		catalogID  = "cid-synced"
		oldVersion = "ver-synced"
		newVersion = "ver-new"
	)

	s := setupRollbackDiscardScenario(t)

	// Fault: SaveConfig's atomic write fails (config.json is now a
	// directory). SaveManifest has already run by the time the write is
	// attempted, so the manifest advances while config stays stale.
	replaceFileWithDir(t, wapi.ConfigPath(s.dir))

	_, err := s.engine.Execute(s.plan)

	require.Error(t, err, "phase 6 must fail when SaveConfig fails")
	assert.Contains(t, err.Error(), "save config",
		"error must come from SaveConfig, not an earlier step")

	// The manifest DID advance (SaveManifest ran first): it records ver-new
	// with the streamed upload hash for a.py and the remote hash for b.py.
	manifest, err := wapi.LoadManifest(s.dir)
	require.NoError(t, err)

	require.NotNil(t, manifest.SyncedVersionID)
	assert.Equal(t, newVersion, *manifest.SyncedVersionID,
		"manifest must be advanced past the failed SaveConfig")
	assert.Equal(t, sha256Hex([]byte(s.newA)), manifest.Files["a.py"].Hash,
		"manifest must record the uploaded a.py hash")
	assert.Equal(t, sha256Hex([]byte(s.remoteB)), manifest.Files["b.py"].Hash,
		"manifest must record the downloaded b.py hash")

	// The rollback dir must be GONE. Discarding only after the state writes
	// strands it here, and the stranded dir is what resurrects stale bytes
	// on the next run.
	assertNoRollbackDir(t, s.rollDir)

	// The working tree keeps the synced bytes: a.py holds the local edit,
	// b.py holds the downloaded remote content. Nothing may restore over it.
	bOnDisk, err := os.ReadFile(filepath.Join(s.dir, "b.py"))
	require.NoError(t, err)

	assert.Equal(t, s.remoteB, string(bOnDisk),
		"b.py must keep the downloaded remote content — no pre-sync restore")

	// Restore config.json with its pre-sync bytes: in the real world the
	// atomic write never landed, so the file keeps its old content.
	cPath := wapi.ConfigPath(s.dir)

	require.NoError(t, os.RemoveAll(cPath))

	// gosec's taint analysis does not flag this write today, so no nolint
	// directive is needed here; if a future gosec flags it again, add one
	// back with the G703 rationale.
	require.NoError(t, os.WriteFile(cPath, s.cfgBytes, 0o644))

	staleCfg, err := wapi.LoadConfig(s.dir)
	require.NoError(t, err)

	require.NotNil(t, staleCfg.LastSyncedVersionID)
	assert.Equal(t, oldVersion, *staleCfg.LastSyncedVersionID,
		"config.json must still hold the pre-sync version — the drift the next run must detect")

	// --- Next run: must reconcile from the remote, not from stale bytes ---

	all, err := s.fake.AllFiles(catalogID, newVersion)
	require.NoError(t, err)

	fake2 := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-2",
		versionID: newVersion,
	}).withVersion(catalogID, newVersion, all)

	// The codeRef now points at ver-new: run 1's PatchCodeRef succeeded
	// before Phase 6 failed, so the artifact genuinely moved ahead.
	e2, err := newWithDeps(s.dir, Options{Yes: true}, Deps{
		Files: fake2,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, newVersion), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e2.Close() })

	plan2, err := e2.Plan()
	require.NoError(t, err)

	// No stale restore: with the rollback discarded at entry, Phase 0 has
	// nothing to resurrect. Pre-fix, this is true AND b.py has been rolled
	// back to its pre-sync bytes.
	assert.False(t, e2.StaleRollbackRestored(),
		"the next run must not restore a stale rollback — the rollback dir was discarded at Phase 6 entry")

	// The plan must not contain false LOCAL_MODIFIED / LOCAL_ADDED uploads
	// for the rollback-covered paths. b.py is the covered path here: its
	// manifest entry (advanced) matches the remote, so there is nothing to
	// upload. Pre-fix, the resurrected pre-sync bytes are classified as
	// LOCAL_MODIFIED and silently re-uploaded over the remote.
	assert.Empty(t, plan2.Uploads,
		"the next run must not upload anything for rollback-covered paths — got %v", plan2.Uploads)
	assert.True(t, plan2.IsEmpty(),
		"the next run's plan must reconcile from the remote and be empty")

	// Executing the (empty) plan runs Phase 6, which converges config to the
	// version the manifest already records — the self-healing direction.
	_, err = e2.Execute(plan2)
	require.NoError(t, err)

	convergedCfg, err := wapi.LoadConfig(s.dir)
	require.NoError(t, err)

	require.NotNil(t, convergedCfg.LastSyncedVersionID)
	assert.Equal(t, newVersion, *convergedCfg.LastSyncedVersionID,
		"config must converge to the manifest's version on the next run")

	// Three-way consistency after convergence: manifest hash == server
	// checksum for every file.
	convergedManifest, err := wapi.LoadManifest(s.dir)
	require.NoError(t, err)

	serverFiles, err := fake2.AllFiles(catalogID, newVersion)
	require.NoError(t, err)

	for path, fm := range convergedManifest.Files {
		serverFM, ok := serverFiles[path]
		require.True(t, ok, "server must hold %s after convergence", path)

		assert.Equal(t, serverFM.Hash, fm.Hash,
			"manifest hash for %s must match the server after convergence", path)
	}
}

// TestPhase6SaveManifestFailure_DiscardsRollback pins the other write-failure
// leg of VAL-ROLLBACK-001: SaveManifest itself fails. The discard-first
// ordering has already removed the rollback dir by then, so no Phase 6
// outcome — not even the earliest write failing — strands it.
func TestPhase6SaveManifestFailure_DiscardsRollback(t *testing.T) {
	s := setupRollbackDiscardScenario(t)

	// Fault: SaveManifest's atomic write fails (manifest.json is now a
	// directory). Neither state file advances past this point.
	replaceFileWithDir(t, filepath.Join(wapi.Dir(s.dir), "manifest.json"))

	_, err := s.engine.Execute(s.plan)

	require.Error(t, err, "phase 6 must fail when SaveManifest fails")
	assert.Contains(t, err.Error(), "save manifest",
		"error must come from SaveManifest, not an earlier step")

	// Discard-first means the rollback dir is already gone even though the
	// very first state write failed.
	assertNoRollbackDir(t, s.rollDir)

	// config.json must not have advanced — SaveConfig runs after the failed
	// SaveManifest.
	cfg, err := wapi.LoadConfig(s.dir)
	require.NoError(t, err)

	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, "ver-synced", *cfg.LastSyncedVersionID,
		"config must not advance when SaveManifest fails")
}

// TestPhase6CleanSuccessDiscardsRollback pins the no-failure leg of
// VAL-ROLLBACK-001 for completeness: a fully successful Phase 6 also leaves
// no rollback dir behind. The interruption tests already assert this for an
// uploads-only plan; this variant covers a plan that also downloads.
func TestPhase6CleanSuccessDiscardsRollback(t *testing.T) {
	s := setupRollbackDiscardScenario(t)

	result, err := s.engine.Execute(s.plan)
	require.NoError(t, err)
	require.NotNil(t, result)

	assertNoRollbackDir(t, s.rollDir)

	// The synced state must reflect the applied plan.
	manifest, err := wapi.LoadManifest(s.dir)
	require.NoError(t, err)

	assert.Equal(t, sha256Hex([]byte(s.newA)), manifest.Files["a.py"].Hash)
	assert.Equal(t, sha256Hex([]byte(s.remoteB)), manifest.Files["b.py"].Hash)
}

// TestPhase6DiscardFailure_AbortsBeforeStateWrites covers the one Phase 6
// failure mode the write-failure tests above cannot reach: Discard itself
// failing. The fault rides Phase 5's final step (PatchCodeRef), which fires
// after the rollback dir exists but before Phase 6 runs, and strips write
// permission from the rollback dir so Discard's os.RemoveAll fails.
//
// Phase 6 must abort with a wrapped error BEFORE SaveManifest/SaveConfig.
// At entry nothing has been persisted, so un-advanced state plus the
// surviving rollback dir is exactly the recoverable mid-Phase-5 outcome:
// the next run's stale-rollback restore puts back pre-sync bytes that the
// un-advanced manifest still matches, and the next diff schedules downloads,
// not false uploads. Swallowing the error instead (the pre-fix behavior)
// strands the rollback dir next to ADVANCED state, and that same stale
// restore resurrects pre-sync bytes as phantom local edits which the next
// sync silently re-uploads over the remote.
func TestPhase6DiscardFailure_AbortsBeforeStateWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fault injection relies on POSIX directory permissions; windows ignores them")
	}

	// Declare first so the hook closure can read s.rollDir: the hook fires
	// during Execute, long after the assignment completes.
	var s *rollbackDiscardScenario

	s = setupRollbackDiscardScenarioWithPatchHook(t, func(_, _, _ string) error {
		// Phase 5's last step: make the rollback dir unremovable so the
		// Phase 6 entry Discard fails. The hook itself must succeed so
		// Phase 5 completes and Phase 6 is genuinely reached.
		if err := os.Chmod(s.rollDir, 0o555); err != nil {
			return fmt.Errorf("fault: chmod rollback dir: %w", err)
		}

		return nil
	})

	// Restore permissions before t.TempDir cleanup so the read-only dir can
	// be removed; cleanup runs LIFO, so this lands ahead of the TempDir one.
	t.Cleanup(func() { _ = os.Chmod(s.rollDir, 0o755) })

	preManifest, err := os.ReadFile(filepath.Join(wapi.Dir(s.dir), "manifest.json"))
	require.NoError(t, err)

	_, err = s.engine.Execute(s.plan)

	require.Error(t, err, "Phase 6 must abort when the rollback dir cannot be discarded")
	assert.Contains(t, err.Error(), "discard rollback",
		"error must be the wrapped Discard failure, not a later write error")

	// No state write may have happened: the manifest stays byte-identical to
	// its pre-sync content, and config does not advance.
	postManifest, err := os.ReadFile(filepath.Join(wapi.Dir(s.dir), "manifest.json"))
	require.NoError(t, err)

	assert.Equal(t, preManifest, postManifest,
		"manifest.json must be untouched when Discard fails at Phase 6 entry")

	cfg, err := wapi.LoadConfig(s.dir)
	require.NoError(t, err)

	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, "ver-synced", *cfg.LastSyncedVersionID,
		"config.json must not advance when Discard fails at Phase 6 entry")

	// The rollback dir survives the failed Discard — that is the fault. Its
	// presence is safe only because no state advanced (asserted above): the
	// next run's stale-rollback recovery restores it against un-advanced
	// state, which is the recoverable mid-Phase-5 outcome.
	_, statErr := os.Stat(s.rollDir)
	assert.NoError(t, statErr, "the unremovable rollback dir must still be present after the abort")
}
