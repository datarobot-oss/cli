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
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preSyncState captures the on-disk state of manifest.json, config.json, and
// the project files so a test can assert nothing was advanced after a failed
// sync. The manifest and config are captured as raw bytes for byte-identical
// comparison; the file contents are captured per-path so a rollback failure
// that leaves a modified file is caught.
type preSyncState struct {
	manifestBytes []byte
	configBytes   []byte
	fileContents  map[string][]byte
}

// captureState reads manifest.json, config.json, and the given project files
// as raw bytes. The caller lists the relative paths whose content matters
// for the rollback assertion.
func captureState(t *testing.T, dir string, files []string) preSyncState {
	t.Helper()

	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	mBytes, err := os.ReadFile(mPath)
	require.NoError(t, err)

	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	contents := make(map[string][]byte, len(files))

	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err)

		contents[rel] = content
	}

	return preSyncState{
		manifestBytes: mBytes,
		configBytes:   cBytes,
		fileContents:  contents,
	}
}

// assertStateUntouched asserts manifest.json and config.json are byte-identical
// to their pre-sync content and every captured project file is unchanged.
// This is the core integrity assertion for every failure mode: a failed sync
// must never advance persisted state.
func assertStateUntouched(t *testing.T, dir string, pre preSyncState) {
	t.Helper()

	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	mBytes, err := os.ReadFile(mPath)
	require.NoError(t, err)

	assert.Equal(t, pre.manifestBytes, mBytes,
		"manifest.json must be byte-identical to its pre-sync content")

	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	assert.Equal(t, pre.configBytes, cBytes,
		"config.json must be byte-identical to its pre-sync content")

	for rel, expected := range pre.fileContents {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err)

		assert.Equal(t, expected, content,
			"project file %s must be unchanged after a failed sync (rollback)", rel)
	}
}

// newSyncedEngine builds an engine over a synced project with the given files
// modified to introduce pending changes. The fake and artifact store are
// returned so the caller can inspect counters and configure fault injection
// before running the engine.
func newSyncedEngine(t *testing.T, files map[string]string, mods map[string]string, fake *fakeFilesClient) (*Engine, string) {
	t.Helper()

	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, files, catalogID, versionID)

	for rel, content := range mods {
		modifyFile(t, dir, rel, content)
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e, dir
}

// --- VAL-UPLOAD-011: Missing streamed hash hard-fails and persists nothing ---

// TestMissingSentEntry_BuildNewBaseManifestHardFails verifies that
// buildNewBaseManifest returns an error naming the missing path when the
// Sent map lacks an entry for an uploaded file. The error must NOT fall back
// to the Phase-2 planned hash — a per-path fallback IS the original poisoning
// bug. This is the unit-level guard; the engine-level guard
// (TestMissingSentEntry_Phase6DoesNotAdvanceManifest) verifies the on-disk
// consequence.
func TestMissingSentEntry_BuildNewBaseManifestHardFails(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid",
			VersionID: "ver",
			Sent:      map[string]FileEntry{}, // missing app.py
		},
	}

	_, err := buildNewBaseManifest(e, "ver", time.Now())

	require.Error(t, err, "missing Sent entry must hard-fail, not fall back")
	assert.Contains(t, err.Error(), "app.py",
		"error must name the missing path")
}

// TestMissingSentEntry_Phase6DoesNotAdvanceManifest verifies that when
// buildNewBaseManifest fails inside phase6State (because Sent is missing an
// entry), neither manifest.json nor config.json is written — both retain
// their exact pre-sync content. This is the on-disk consequence of the
// hard-fail combined with the manifest-before-config write ordering.
//
// The invariant: config never moves ahead of the manifest. Phase 6 now
// builds and writes the manifest first, then writes config. When
// buildNewBaseManifest fails, neither write has occurred, so config stays
// at the old version. The missing-Sent scenario is not reachable through
// normal operation (uploadFilesParallel returns an error if any upload
// fails, so a partial Sent never reaches Phase 6), but the ordering
// invariant is what protects the reachable hazard (SaveManifest I/O
// failure), so this test guards it directly.
func TestMissingSentEntry_Phase6DoesNotAdvanceManifest(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	pre := captureState(t, dir, []string{"app.py"})

	// Construct an engine in the state Phase 6 would see after a successful
	// Phase 5 where the Sent map is missing app.py. This is not reachable
	// through the engine (uploadFilesParallel returns an error on any upload
	// failure), so we test phase6State directly.
	e := &Engine{
		projectDir: dir,
		config:     cfg,
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid-new",
			VersionID: "ver-new",
			Sent:      map[string]FileEntry{}, // missing app.py
		},
		newCatalogID: "cid-new",
		newVersionID: "ver-new",
		nowFn:        time.Now,
	}

	err = phase6State(e)

	require.Error(t, err, "phase6State must fail when Sent is missing")
	assert.Contains(t, err.Error(), "app.py",
		"error must name the missing path")

	// manifest.json must NOT be written — SaveManifest runs after
	// buildNewBaseManifest, which failed.
	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	mBytes, err := os.ReadFile(mPath)
	require.NoError(t, err)

	assert.Equal(t, pre.manifestBytes, mBytes,
		"manifest.json must be byte-identical to its pre-sync content")

	// config.json must NOT be advanced. Phase 6 now writes the manifest
	// before config: buildNewBaseManifest runs first, and when it fails,
	// neither SaveManifest nor SaveConfig has executed. Config stays at
	// the old version, preserving the invariant that config never moves
	// ahead of the manifest.
	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	assert.Equal(t, pre.configBytes, cBytes,
		"config.json must be byte-identical to its pre-sync content — config never moves ahead of the manifest")

	// Config must still point at the old version, proving the invariant.
	unchangedCfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, unchangedCfg.LastSyncedVersionID)
	assert.Equal(t, "ver-synced", *unchangedCfg.LastSyncedVersionID,
		"config LastSyncedVersionID must NOT be advanced when the manifest build fails")

	// The project file must be unchanged (Phase 6 does not modify the tree).
	content, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err)

	assert.Equal(t, pre.fileContents["app.py"], content,
		"app.py must be unchanged")
}

// TestMissingSentEntry_DoesNotFallBackToPhase2Hash verifies that the error
// from buildNewBaseManifest does not silently produce a manifest with the
// Phase-2 hash. The manifest is not written at all, so there is no entry to
// check — but we also verify the returned manifest is the zero value so no
// caller can accidentally use it.
func TestMissingSentEntry_DoesNotFallBackToPhase2Hash(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid",
			VersionID: "ver",
			Sent:      map[string]FileEntry{},
		},
	}

	manifest, err := buildNewBaseManifest(e, "ver", time.Now())

	require.Error(t, err)
	assert.Empty(t, manifest.Files,
		"no manifest must be produced when Sent is missing — no fallback")
}

// --- Phase 6 write-ordering invariants ---

// TestSaveManifestFailure_DoesNotAdvanceConfig verifies that when SaveManifest
// fails inside phase6State (injected by making manifest.json a directory so
// the atomic rename fails), config.json is NOT advanced. Phase 6 now writes
// the manifest before config: SaveManifest runs first, and when it fails,
// SaveConfig has not executed. Config stays at the old version, preserving
// the invariant that config never moves ahead of the manifest.
//
// This is the reachable hazard (SaveManifest I/O failure) that the write
// reorder fixes. A stale config paired with an un-advanced manifest makes
// the next sync detect drift, fetch AllFiles, and rebuild BASE from real
// remote data — safe and self-healing. The converse (advanced config,
// stale manifest) silently poisons BASE.
func TestSaveManifestFailure_DoesNotAdvanceConfig(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	pre := captureState(t, dir, []string{"app.py"})

	// Make manifest.json a directory so AtomicWriteFile's rename fails.
	// SaveManifest creates a temp file in the parent dir, then renames it
	// over the target — renaming a file over a directory fails with EISDIR.
	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	require.NoError(t, os.Remove(mPath))
	require.NoError(t, os.Mkdir(mPath, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(mPath) })

	e := &Engine{
		projectDir: dir,
		config:     cfg,
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{
			"app.py": {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
		},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid-new",
			VersionID: "ver-new",
			Sent: map[string]FileEntry{
				"app.py": {Hash: sha256Hex([]byte("print('changed')\n")), Size: 14},
			},
		},
		newCatalogID: "cid-new",
		newVersionID: "ver-new",
		nowFn:        time.Now,
	}

	err = phase6State(e)

	require.Error(t, err, "phase6State must fail when SaveManifest fails")
	assert.Contains(t, err.Error(), "save manifest",
		"error must come from SaveManifest, not SaveConfig")

	// config.json must NOT be advanced — SaveConfig runs after SaveManifest,
	// which failed, so config was never written.
	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	assert.Equal(t, pre.configBytes, cBytes,
		"config.json must be byte-identical to its pre-sync content — config never moves ahead of the manifest")

	unchangedCfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, unchangedCfg.LastSyncedVersionID)
	assert.Equal(t, "ver-synced", *unchangedCfg.LastSyncedVersionID,
		"config LastSyncedVersionID must NOT be advanced when SaveManifest fails")
}

// TestSaveConfigFailure_ManifestAdvanced_ConfigConvergesOnNextRealSync
// verifies the safe failure direction and the honest recovery path: when
// SaveManifest succeeds but SaveConfig fails (injected by making config.json
// a directory so the atomic rename fails), the manifest IS advanced (new
// version, streamed hashes) while config stays stale (old version).
//
// What production then does — and all this test asserts — is asymmetric on
// purpose. The next sync detects drift (config's old version vs the
// artifact's new one), fetches AllFiles rather than fast-pathing, computes an
// EMPTY plan (the advanced manifest matches the remote, and the disk matches
// both), and returns WITHOUT executing: Run short-circuits empty plans before
// Execute, so Phase 6 never runs and config.json keeps the old version. No
// production caller reaches Execute with an empty plan, so a test that forces
// one there would assert a convergence path that cannot happen. Config is
// converged only by the next sync that has real work to do — which is exactly
// what the second half of this test drives and asserts.
//
// The asymmetry — manifest advanced past a stale config — is the safe
// direction, and the reason manifest-before-config is the correct write
// order: the version mismatch makes EVERY later sync detect drift and fetch
// the real remote, so the window self-heals at the first sync with actual
// work. The reverse — config advanced past a stale manifest — is silent,
// permanent BASE poisoning: no drift is ever detected, the fast path copies
// the stale BASE to REMOTE, and the CLI reports "Up to date." forever.
func TestSaveConfigFailure_ManifestAdvanced_ConfigConvergesOnNextRealSync(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
		newVerID  = "ver-new"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	pre := captureState(t, dir, []string{"app.py"})

	// Make config.json a directory so SaveConfig's atomic rename fails.
	// SaveManifest writes to manifest.json (a regular file), so it succeeds;
	// SaveConfig writes to config.json (now a directory), so it fails.
	cPath := wapi.ConfigPath(dir)

	require.NoError(t, os.Remove(cPath))
	require.NoError(t, os.Mkdir(cPath, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(cPath) })

	streamedContent := "print('changed')\n"
	streamedHash := sha256Hex([]byte(streamedContent))
	streamedSize := int64(len(streamedContent))

	// Modify the disk file to match what was "uploaded" so that after
	// SaveManifest writes the streamed hash, local == base and the next
	// sync's plan is empty (the point is drift detection, not finding work).
	modifyFile(t, dir, "app.py", streamedContent)

	// Include .drignore in remote so the manifest carries it through —
	// buildNewBaseManifest seeds from remote, then overrides uploaded paths.
	ignoreHash, ignoreSize, err := hashLocal(t, dir, ignore.FileName)
	require.NoError(t, err)

	e := &Engine{
		projectDir: dir,
		config:     cfg,
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{
			"app.py":        {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
			ignore.FileName: {Hash: ignoreHash, Size: ignoreSize},
		},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid-new",
			VersionID: newVerID,
			Sent: map[string]FileEntry{
				"app.py": {Hash: streamedHash, Size: streamedSize},
			},
		},
		newCatalogID: "cid-new",
		newVersionID: newVerID,
		nowFn:        time.Now,
	}

	err = phase6State(e)

	require.Error(t, err, "phase6State must fail when SaveConfig fails")
	assert.Contains(t, err.Error(), "save config",
		"error must come from SaveConfig, not SaveManifest")

	// manifest.json IS advanced — SaveManifest ran before SaveConfig and
	// succeeded. The manifest now carries the new version and the streamed hash.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, wapi.ManifestVersion, manifest.Version)

	require.NotNil(t, manifest.SyncedVersionID)
	assert.Equal(t, newVerID, *manifest.SyncedVersionID,
		"manifest syncedVersionId must be advanced")

	fm, ok := manifest.Files["app.py"]
	require.True(t, ok)

	assert.Equal(t, streamedHash, fm.Hash,
		"manifest must carry the streamed hash")
	assert.Equal(t, streamedSize, fm.Size,
		"manifest must carry the streamed size")

	// Restore config.json with the old content so the next sync can load it.
	// This simulates the real-world state after a SaveConfig failure: the
	// file retains its pre-sync content because the atomic write never landed.
	require.NoError(t, os.RemoveAll(cPath))
	require.NoError(t, os.WriteFile(cPath, pre.configBytes, 0o644))

	// Build the fake's server state from the manifest that SaveManifest wrote,
	// so AllFiles returns exactly what BASE describes. This ensures the next
	// sync sees base == remote and the plan is empty — the point is that the
	// sync runs the full pipeline (drift detection, AllFiles fetch) rather
	// than fast-pathing, not that it finds work to do.
	serverFiles := make(map[string]filesapi.FileMeta, len(manifest.Files))

	for path, fm := range manifest.Files {
		serverFiles[path] = filesapi.FileMeta{Hash: fm.Hash, Size: fm.Size}
	}

	// The next sync goes through Run() — the production entry point, where
	// the plan and the execute decision both live. The fake's server state
	// is built from the manifest SaveManifest wrote, so AllFiles serves
	// exactly what BASE describes: base == remote, and the disk matches
	// both, so the honest plan is empty. What must still happen is the
	// round-trip — drift was detected, the remote was fetched, no silent
	// fast path. What must NOT happen is Execute: no production caller
	// executes an empty plan.
	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-should-not-be-used",
		versionID: newVerID,
	}).withVersion(catalogID, newVerID, serverFiles)

	// One engine per sync invocation, matching how the command layer
	// constructs a fresh engine for every run.
	newEngine := func() (*Engine, error) {
		return newWithDeps(dir, Options{Yes: true}, Deps{
			Files: fake,
			Artifacts: &fakeArtifactStore{
				GetFn: func(id string) (*workload.Artifact, error) {
					return draftArtifact(id, catalogID, newVerID), nil
				},
				PatchFn: func(_, _, _ string) error { return nil },
			},
			Now: time.Now,
		})
	}

	e2, err := newEngine()
	require.NoError(t, err)

	t.Cleanup(func() { _ = e2.Close() })

	result, err := e2.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Drift was detected: AllFiles was called (not fast-pathed).
	assert.Equal(t, 1, fake.AllFilesCalls(),
		"drift must trigger an AllFiles round-trip, not the fast path")

	// The plan Run computed is empty: manifest (advanced) == remote
	// (AllFiles), and local == base (no disk changes since the failed
	// sync). The sync ran the full pipeline — it did not silently
	// fast-path — it just found nothing to do.
	assert.True(t, e2.plan.IsEmpty(),
		"plan should be empty — manifest matches remote, no disk changes")

	// Run returned WITHOUT executing. The empty-plan short-circuit fires
	// before Execute, so no upload-side call was issued at all.
	assert.Equal(t, 0, fake.CreateStageCalls(),
		"the empty-plan sync must not stage anything — Run returns before Execute")
	assert.Equal(t, 0, fake.UploadToStageCalls(),
		"the empty-plan sync must not upload anything — Run returns before Execute")
	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"the empty-plan sync must not apply anything — Run returns before Execute")

	assert.Empty(t, result.NewVersion,
		"the empty-plan run creates no new version — Phase 6 never ran")

	// Phase 6 never ran, so config.json still holds the OLD version while
	// manifest.json holds the new one. That asymmetry is the safe one (see
	// the function comment): the version mismatch makes every later sync
	// detect drift and fetch the real remote, so the window self-heals at
	// the first sync with actual work. The reverse asymmetry — config
	// advanced past a stale manifest — would never be detected and would
	// silently poison BASE forever.
	staleCfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, staleCfg.LastSyncedVersionID)
	assert.Equal(t, versionID, *staleCfg.LastSyncedVersionID,
		"config must still hold the old version — Run short-circuits the empty plan, so Phase 6 never runs")

	advManifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	require.NotNil(t, advManifest.SyncedVersionID)
	assert.Equal(t, newVerID, *advManifest.SyncedVersionID,
		"manifest stays advanced — SaveManifest ran before the SaveConfig failure")

	// --- Convergence needs a sync with real work ---

	// Only a plan with actual work makes Run reach Execute, and only the
	// Phase 6 reached that way converges config. Introduce a real change
	// so the next run has something to upload.
	finalContent := "print('real work')\n"

	modifyFile(t, dir, "app.py", finalContent)

	e3, err := newEngine()
	require.NoError(t, err)

	t.Cleanup(func() { _ = e3.Close() })

	result3, err := e3.Run()
	require.NoError(t, err)
	require.NotNil(t, result3)

	require.Len(t, e3.plan.Uploads, 1,
		"the modified file must be the one real upload")
	assert.Equal(t, "app.py", e3.plan.Uploads[0].Path)
	assert.Equal(t, 1, result3.UploadedCount,
		"the real-work sync must report the upload it performed")

	// Config still lagged the artifact, so this run detected drift too and
	// fetched AllFiles a second time.
	assert.Equal(t, 2, fake.AllFilesCalls(),
		"the still-drifted run must fetch AllFiles again")

	assert.Equal(t, 1, fake.ApplyStageCalls(),
		"the real-work sync must execute its plan")

	// Config must now converge to the manifest's version — the honest
	// self-healing path: a sync with real work, never a forced Execute.
	convergedCfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, convergedCfg.LastSyncedVersionID)
	assert.Equal(t, newVerID, *convergedCfg.LastSyncedVersionID,
		"config must converge to the manifest's version once a sync has real work")

	convergedManifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	require.NotNil(t, convergedManifest.SyncedVersionID)
	assert.Equal(t, *convergedCfg.LastSyncedVersionID, *convergedManifest.SyncedVersionID,
		"config and manifest must agree on the version after convergence")

	// The converged manifest records the bytes this sync actually
	// streamed, and the server holds exactly those bytes.
	convergedFM, ok := convergedManifest.Files["app.py"]
	require.True(t, ok)

	assert.Equal(t, sha256Hex([]byte(finalContent)), convergedFM.Hash,
		"manifest must record the hash streamed by the converging sync")

	serverAll, err := fake.AllFiles(catalogID, newVerID)
	require.NoError(t, err)

	serverFM, ok := serverAll["app.py"]
	require.True(t, ok)

	assert.Equal(t, convergedFM.Hash, serverFM.Hash,
		"the server must hold the bytes the manifest records after convergence")
}

// --- VAL-UPLOAD-011b / VAL-UPLOAD-013: Failure modes through the engine ---

// TestPartialSent_WorkerError_FailsCleanly verifies that when some workers
// succeed and one errors (producing a partial Sent internally), the sync
// fails, Phase 6 never runs, and manifest.json and config.json retain their
// pre-sync content. The partial Sent is never merged with a fallback because
// uploadFilesParallel returns an error, not a partial map.
// Fulfills VAL-UPLOAD-011 (partial Sent) and VAL-UPLOAD-013.
func TestPartialSent_WorkerError_FailsCleanly(t *testing.T) {
	files := map[string]string{
		"a.py": "aaa\n",
		"b.py": "bbb\n",
		"c.py": "ccc\n",
	}

	mods := map[string]string{
		"a.py": "AAA\n",
		"b.py": "BBB\n",
		"c.py": "CCC\n",
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-synced",
		stageID:   "stage-1",
		versionID: "ver-new",
	}).withFailNthUpload(2)

	e, dir := newSyncedEngine(t, files, mods, fake)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Len(t, plan.Uploads, 3, "all three files should be pending uploads")

	pre := captureState(t, dir, []string{"a.py", "b.py", "c.py"})

	_, err = e.Execute(plan)

	require.Error(t, err, "sync must fail when a worker errors")
	assert.Contains(t, err.Error(), "upload",
		"error must come from the upload step")

	assertStateUntouched(t, dir, pre)

	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"ApplyStage must not be called when an upload fails — no partial apply")
}

// TestUploadFailure_OneFileFailsLate_FailsCleanly verifies that when one file
// fails after others have been uploaded to the staging area, the sync fails,
// Phase 6 never runs, and all persisted state is untouched. Files staged but
// never applied must not appear in the manifest.
// Fulfills VAL-UPLOAD-013.
func TestUploadFailure_OneFileFailsLate_FailsCleanly(t *testing.T) {
	files := map[string]string{
		"a.py": "aaa\n",
		"b.py": "bbb\n",
		"c.py": "ccc\n",
		"d.py": "ddd\n",
		"e.py": "eee\n",
	}

	mods := map[string]string{
		"a.py": "AAAA\n",
		"b.py": "BBBB\n",
		"c.py": "CCCC\n",
		"d.py": "DDDD\n",
		"e.py": "EEEE\n",
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-synced",
		stageID:   "stage-1",
		versionID: "ver-new",
	}).withFailNthUpload(4)

	e, dir := newSyncedEngine(t, files, mods, fake)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Len(t, plan.Uploads, 5, "all five files should be pending uploads")

	pre := captureState(t, dir, []string{"a.py", "b.py", "c.py", "d.py", "e.py"})

	_, err = e.Execute(plan)

	require.Error(t, err, "sync must fail when one file fails late")
	assert.Contains(t, err.Error(), "upload",
		"error must come from the upload step")

	assertStateUntouched(t, dir, pre)

	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"ApplyStage must not be called — staged-but-unapplied files must not reach the manifest")
}

// TestUploadFailure_AllFilesFail_FailsCleanly verifies that when every upload
// fails, the sync fails immediately, Phase 6 never runs, and all persisted
// state is untouched.
// Fulfills VAL-UPLOAD-013.
func TestUploadFailure_AllFilesFail_FailsCleanly(t *testing.T) {
	files := map[string]string{
		"app.py": "print('orig')\n",
	}

	mods := map[string]string{
		"app.py": "print('changed')\n",
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-synced",
		stageID:   "stage-1",
		versionID: "ver-new",
	}).withFailNthUpload(1)

	e, dir := newSyncedEngine(t, files, mods, fake)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Len(t, plan.Uploads, 1, "one file should be pending upload")

	pre := captureState(t, dir, []string{"app.py"})

	_, err = e.Execute(plan)

	require.Error(t, err, "sync must fail when all uploads fail")
	assert.Contains(t, err.Error(), "app.py",
		"error must name the failing path")

	assertStateUntouched(t, dir, pre)

	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"ApplyStage must not be called when all uploads fail")
}

// TestUploadFailure_ApplyStageFails_FailsCleanly verifies that when all files
// are staged successfully but ApplyStage fails, the sync fails, Phase 6
// never runs, and all persisted state is untouched. Files staged but never
// applied must not appear in the manifest.
// Fulfills VAL-UPLOAD-013.
func TestUploadFailure_ApplyStageFails_FailsCleanly(t *testing.T) {
	files := map[string]string{
		"a.py": "aaa\n",
		"b.py": "bbb\n",
	}

	mods := map[string]string{
		"a.py": "AAAA\n",
		"b.py": "BBBB\n",
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-synced",
		stageID:   "stage-1",
		versionID: "ver-new",
	}).withFailApplyStage()

	e, dir := newSyncedEngine(t, files, mods, fake)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Len(t, plan.Uploads, 2, "both files should be pending uploads")

	pre := captureState(t, dir, []string{"a.py", "b.py"})

	_, err = e.Execute(plan)

	require.Error(t, err, "sync must fail when ApplyStage fails")
	assert.Contains(t, err.Error(), "apply stage",
		"error must come from the apply-stage step")

	assertStateUntouched(t, dir, pre)

	// ApplyStage WAS called (it failed), but the resulting version must not
	// be persisted. The manifest must not record hashes for staged-but-
	// unapplied files.
	assert.Equal(t, 1, fake.ApplyStageCalls(),
		"ApplyStage must be called (it failed)")
}

// --- VAL-UPLOAD-021: File deleted between Plan and Execute ---

// TestDeletedFileBetweenPlanAndExecute_FailsCleanly verifies that a planned
// upload file deleted from disk between Plan() and Execute() produces an error
// naming that path (no panic), with manifest and config unchanged, rollback
// performed, and remaining planned files not applied. The first error stops
// the pipeline: ApplyStage is never called.
// Fulfills VAL-UPLOAD-021.
func TestDeletedFileBetweenPlanAndExecute_FailsCleanly(t *testing.T) {
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

	fake := &fakeFilesClient{
		catalogID: "cid-synced",
		stageID:   "stage-1",
		versionID: "ver-new",
	}

	e, dir := newSyncedEngine(t, files, mods, fake)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.Len(t, plan.Uploads, 3, "all three files should be pending uploads")

	// Capture the state of the files that will NOT be deleted.
	pre := captureState(t, dir, []string{"a.py", "c.py"})

	// Between Plan and Execute, delete b.py from disk. os.Open will return
	// os.ErrNotExist when the uploader tries to read it.
	require.NoError(t, os.Remove(filepath.Join(dir, "b.py")))

	_, err = e.Execute(plan)

	require.Error(t, err, "sync must fail when a planned file is missing")
	assert.Contains(t, err.Error(), "b.py",
		"error must name the deleted path")
	assert.NotContains(t, err.Error(), "panic",
		"error must not be a panic")

	// manifest.json and config.json must be byte-identical to pre-sync.
	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	mBytes, err := os.ReadFile(mPath)
	require.NoError(t, err)

	assert.Equal(t, pre.manifestBytes, mBytes,
		"manifest.json must be byte-identical to its pre-sync content")

	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	assert.Equal(t, pre.configBytes, cBytes,
		"config.json must be byte-identical to its pre-sync content")

	// The remaining files (not deleted by the test) must be unchanged —
	// the rollback restored the working tree to its pre-Execute state.
	for _, rel := range []string{"a.py", "c.py"} {
		content, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err)

		assert.Equal(t, pre.fileContents[rel], content,
			"file %s must be unchanged after the failed sync", rel)
	}

	// ApplyStage must not be called — the first error stops the pipeline.
	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"ApplyStage must not be called when a planned file is missing")
}
