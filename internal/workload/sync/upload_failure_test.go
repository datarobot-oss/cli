// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except compliance with the License.
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

	"github.com/datarobot/cli/internal/workload"
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
// entry), manifest.json is not written — it retains its exact pre-sync
// content. This is the on-disk consequence of the hard-fail.
//
// NOTE: This test also exposes the SaveConfig-before-SaveManifest ordering
// hazard: SaveConfig runs before buildNewBaseManifest, so config.json IS
// advanced even though the manifest is not. The missing-Sent scenario is not
// reachable through normal operation (uploadFilesParallel returns an error
// if any upload fails, so a partial Sent never reaches Phase 6), but the
// hazard is real and documented in architecture.md section 3.2. See
// discoveredIssues in the handoff.
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

	// config.json: SaveConfig runs BEFORE buildNewBaseManifest in phase6State.
	// When buildNewBaseManifest fails, SaveConfig has already persisted the
	// advanced config (LastSyncedVersionID = "ver-new"). This is the
	// SaveConfig-before-SaveManifest ordering hazard documented in
	// architecture.md section 3.2. The missing-Sent scenario is not reachable
	// through normal operation (uploadFilesParallel returns an error on any
	// upload failure, so a partial Sent never reaches Phase 6), so this
	// hazard is theoretical. We assert the current behaviour and document
	// the gap; see discoveredIssues in the handoff.
	cBytes, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	// If the hazard were fixed (SaveConfig moved after buildNewBaseManifest,
	// or made conditional on manifest success), this would assert equality.
	// Today config IS advanced, so we assert the difference and name it.
	assert.NotEqual(t, pre.configBytes, cBytes,
		"config.json IS advanced by SaveConfig before buildNewBaseManifest fails (known hazard)")

	// The advanced config must point at the new version, proving the hazard
	// is specifically the SaveConfig-before-SaveManifest ordering.
	advancedCfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, advancedCfg.LastSyncedVersionID)
	assert.Equal(t, "ver-new", *advancedCfg.LastSyncedVersionID,
		"config LastSyncedVersionID must be advanced (the hazard)")

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
