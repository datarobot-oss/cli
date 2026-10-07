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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// poisonManifestHash overwrites the recorded hash of one path in the
// project's manifest.json, simulating a poisoned BASE: recorded state that
// describes bytes the server does not hold. This is the state --verify
// exists to detect and repair.
func poisonManifestHash(t *testing.T, dir, rel, hash string) {
	t.Helper()

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	fm, ok := m.Files[rel]
	require.True(t, ok, "path %s must exist in the manifest to be poisoned", rel)

	fm.Hash = hash
	m.Files[rel] = fm

	require.NoError(t, wapi.SaveManifest(dir, m))
}

// statePaths returns the two files whose bytes define the persisted sync
// state: manifest.json and config.json under the workload state directory.
func statePaths(dir string) (manifestPath, configPath string) {
	return filepath.Join(dir, ".datarobot", "workload", "manifest.json"),
		filepath.Join(dir, ".datarobot", "workload", "config.json")
}

// readStateFile reads one persisted state file's raw bytes for byte-level
// comparison across runs.
func readStateFile(t *testing.T, path string) []byte {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return b
}

// TestEngine_Run_VerifyRepairsPoisonedManifestOnConvergedEmptyPlan is the
// sharpest repair case: BASE is poisoned to A while disk and server both hold
// B. Classify sees localChanged and remoteChanged with local == remote, so
// the classification is CONVERGED, every row is skipped, and the plan comes
// back empty. The divergence notice must still fire, the run must still exit
// cleanly, and Phase 6 must still rewrite the manifest from the real remote
// — an empty plan must not short-circuit the state write that repairs it.
func TestEngine_Run_VerifyRepairsPoisonedManifestOnConvergedEmptyPlan(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	// Disk holds B and the manifest agrees; poison only the recorded hash.
	dir := syncedProject(t, map[string]string{"app.py": contentB}, catalogID, versionID)

	poisonManifestHash(t, dir, "app.py", sha256Hex([]byte(contentA)))

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": contentB,
	}, nil))

	manifestPath, _ := statePaths(dir)
	before, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var (
		out    string
		runErr error
	)

	out = captureWarnLog(t, func() {
		e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

		_, runErr = e.Run()
	})

	// The divergence is a diagnostic: exit 0, notice on the warn stream.
	require.NoError(t, runErr, "a detected divergence must not change the exit status")
	assert.Contains(t, out, "divergence", "the divergence notice must fire even though the plan is empty")
	assert.Contains(t, out, "app.py", "the divergence notice must name the affected path")
	assert.Contains(t, out, sha256Hex([]byte(contentB)), "the notice must carry the server's real hash")

	// Repair: the manifest hash for app.py is the server's checksum (B),
	// not the poisoned A.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, sha256Hex([]byte(contentB)), manifest.Files["app.py"].Hash,
		"Phase 6 must seed the manifest from the real REMOTE, not from the poisoned BASE")
	assert.NotEqual(t, sha256Hex([]byte(contentA)), manifest.Files["app.py"].Hash,
		"the poisoned hash must not survive the run")

	server, err := fake.AllFiles(catalogID, versionID)
	require.NoError(t, err)
	assert.Equal(t, server["app.py"].Hash, manifest.Files["app.py"].Hash,
		"the repaired manifest hash must equal the server's AllFiles checksum")

	// The repair is a local state write only: no upload-side network calls,
	// no new version.
	assert.Zero(t, fake.CreateStageCalls())
	assert.Zero(t, fake.UploadToStageCalls())
	assert.Zero(t, fake.ApplyStageCalls())
	assert.Zero(t, fake.UploadFromZipCalls())

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, versionID, *cfg.LastSyncedVersionID, "the repair must not advance the synced version")

	// Idempotent: a second --verify run finds no divergence and rewrites
	// nothing. The short-circuit must stay in charge once the manifest is
	// truthful, so the file is byte-identical afterwards.
	afterRepair := readStateFile(t, manifestPath)

	var out2 string

	e2 := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	out2 = captureWarnLog(t, func() {
		_, runErr = e2.Run()
	})

	require.NoError(t, runErr)
	assert.Empty(t, e2.Divergences(), "the second verify run must find no divergence")
	assert.NotContains(t, out2, "divergence", "the second verify run must be silent")
	assert.Equal(t, afterRepair, readStateFile(t, manifestPath),
		"the second verify run must not rewrite the manifest")

	// The manifest must not have been rewritten back to the poison, and the
	// repair itself must have changed exactly the hash field's story: the
	// pre-run bytes recorded A, the post-run bytes record B.
	assert.NotEqual(t, before, afterRepair, "the repair run must rewrite the manifest")

	// A plain sync is now truthful: with the repaired manifest describing
	// the server, the fast path yields an empty plan.
	e3 := engineFor(t, dir, Options{}, fake, catalogID, versionID)

	plan, err := e3.Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty(), "a plain sync must report Up to date. after the repair")
}

// TestEngine_Run_VerifyRepairsWhenDivergentPathIsAlsoDeleted covers the
// delete-shaped empty plan: one path is a CONVERGED skip (poisoned hash, disk
// and server agree) and another is BOTH_DELETED (gone from disk and server,
// recorded only in the poisoned BASE). Both classify to skip, so the plan is
// empty — yet the run must still report both divergences and drop the
// phantom path from the manifest.
func TestEngine_Run_VerifyRepairsWhenDivergentPathIsAlsoDeleted(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	dir := syncedProject(t, map[string]string{
		"app.py":  contentB,
		"gone.py": "gone from both sides\n",
	}, catalogID, versionID)

	// The server lost gone.py too, so its listing holds app.py and .drignore
	// only. The seed is built while gone.py is still on disk (the helper
	// reads the tree), then the path is dropped from it.
	seed := seededServerContents(t, dir, map[string]string{
		"app.py":  contentB,
		"gone.py": "gone from both sides\n",
	}, nil)
	delete(seed, "gone.py")

	// gone.py is deleted locally as well.
	require.NoError(t, os.Remove(filepath.Join(dir, "gone.py")))

	// app.py's recorded hash is poisoned to A while disk and server hold B.
	poisonManifestHash(t, dir, "app.py", sha256Hex([]byte(contentA)))

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	var (
		out    string
		runErr error
	)

	out = captureWarnLog(t, func() {
		e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

		_, runErr = e.Run()
	})

	require.NoError(t, runErr)

	// Both divergent paths are named: the hash mismatch and the phantom path.
	assert.Contains(t, out, "app.py")
	assert.Contains(t, out, "gone.py", "a BASE-only divergence must be reported even when deletion is already agreed")

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, sha256Hex([]byte(contentB)), manifest.Files["app.py"].Hash)

	_, ok := manifest.Files["gone.py"]
	assert.False(t, ok, "a path absent from the server must be dropped from the manifest, not kept because the plan was empty")

	assert.Zero(t, fake.UploadToStageCalls(), "the repair makes no upload-side calls")
}

// TestEngine_Run_VerifyRepairsWhenServerEditedDeletedPath: the divergent
// path is one the user deleted locally while the server edited it (BASE
// records the old hash, so the divergence is visible under --verify). The
// plan carries the download-over-delete row, remote wins, and after the run
// the restored file, the manifest, and the server all agree.
func TestEngine_Run_VerifyRepairsWhenServerEditedDeletedPath(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	// The server seed is captured while the file is still on disk (the
	// helper reads the tree), with B overriding A; then the local deletion.
	seed := seededServerContents(t, dir, map[string]string{
		"app.py": contentA,
	}, map[string][]byte{"app.py": []byte(contentB)})

	// The user deleted the file locally; the server moved on to B.
	require.NoError(t, os.Remove(filepath.Join(dir, "app.py")))

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	var (
		out    string
		runErr error
	)

	out = captureWarnLog(t, func() {
		e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

		_, runErr = e.Run()
	})

	require.NoError(t, runErr)

	assert.Contains(t, out, "divergence", "the stale BASE hash must be reported even though the path is locally deleted")
	assert.Contains(t, out, "app.py")

	b, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err, "remote wins over the local deletion, so the file is restored from the server")
	assert.Equal(t, contentB, string(b))

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex([]byte(contentB)), manifest.Files["app.py"].Hash,
		"the manifest must record the server's bytes after the repair")

	assert.Zero(t, fake.UploadToStageCalls(), "the repair makes no upload-side calls")
}

// TestEngine_Run_VerifyRepairsFlagshipDivergence is the VAL-VERIFY-004 shape
// at the engine level: disk and manifest both hold A, the server holds B, so
// the plan carries a download row. After the run the disk and the manifest
// both describe the server, a second --verify finds nothing, and a plain
// sync reports Up to date.
func TestEngine_Run_VerifyRepairsFlagshipDivergence(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": contentA,
	}, map[string][]byte{"app.py": []byte(contentB)}))

	manifestPath, _ := statePaths(dir)

	e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, fake.DownloadFileCalls(), "the reconciling row is a download")

	b, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err)
	assert.Equal(t, contentB, string(b), "the download must land the server's bytes")

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex([]byte(contentB)), manifest.Files["app.py"].Hash,
		"the manifest must record the server's checksum after the repair")

	afterRepair := readStateFile(t, manifestPath)

	e2 := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	_, err = e2.Run()
	require.NoError(t, err)
	assert.Empty(t, e2.Divergences(), "the second verify run must find no divergence")
	assert.Equal(t, afterRepair, readStateFile(t, manifestPath),
		"the second verify run must not rewrite the manifest")

	e3 := engineFor(t, dir, Options{}, fake, catalogID, versionID)

	plan, err := e3.Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty(), "a plain sync must report Up to date. after the repair")
}

// TestEngine_Plan_VerifyConflictDivergencePreviewsWithoutWriting: a
// both-sides conflict on a path whose BASE is also stale. The preview must
// surface the divergence and the conflict row while writing nothing — no
// manifest rewrite, no .LOCAL copy, disk untouched.
func TestEngine_Plan_VerifyConflictDivergencePreviewsWithoutWriting(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"
	contentC := "print('C')\n"

	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	// Local moves to C while the server moved to B: three-way conflict, and
	// BASE (A) diverges from the server (B) too.
	modifyFile(t, dir, "app.py", contentC)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": contentA,
	}, map[string][]byte{"app.py": []byte(contentB)}))

	manifestPath, _ := statePaths(dir)
	manifestBefore := readStateFile(t, manifestPath)

	var (
		out     string
		planErr error
	)

	out = captureWarnLog(t, func() {
		e := engineFor(t, dir, Options{Verify: true, DryRun: true}, fake, catalogID, versionID)

		var plan *SyncPlan

		plan, planErr = e.Plan()
		require.NoError(t, planErr)

		require.Len(t, plan.Conflicts, 1, "the conflict row must be in the plan")
		assert.Equal(t, "app.py", plan.Conflicts[0].Path)
	})

	require.NoError(t, planErr)
	assert.Contains(t, out, "divergence", "the preview must surface the stale BASE")
	assert.Contains(t, out, "app.py")

	assert.Equal(t, manifestBefore, readStateFile(t, manifestPath),
		"a preview must not rewrite the manifest")

	b, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err)
	assert.Equal(t, contentC, string(b), "a preview must not touch the working tree")

	copies, err := filepath.Glob(filepath.Join(dir, "*.LOCAL.*"))
	require.NoError(t, err)
	assert.Empty(t, copies, "a preview must not create conflict copies")
}

// TestEngine_Run_VerifyOnLockedArtifactFails: --verify must not bypass the
// locked-artifact check. A non-dry-run verify fails with the same locked
// error a plain sync produces.
func TestEngine_Run_VerifyOnLockedArtifactFails(t *testing.T) {
	dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

	e := lockedEngine(t, dir, Options{Verify: true})

	_, err := e.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked")
}

// TestEngine_Plan_VerifyDryRunOnLockedArtifactIsPreview: --verify --dry-run
// on a locked artifact still plans, with the locked notice set — the
// preview exemption must survive --verify.
func TestEngine_Plan_VerifyDryRunOnLockedArtifactIsPreview(t *testing.T) {
	dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

	e := lockedEngine(t, dir, Options{Verify: true, DryRun: true})

	plan, err := e.Plan()
	require.NoError(t, err)
	require.NotNil(t, plan)

	assert.Contains(t, e.LockedNotice(), "locked",
		"a verify preview against a locked artifact must still carry the locked notice")
}

// TestEngine_Run_DefaultPathDoesNotRepairPoison pins the documented residual
// risk: without --verify the poisoned manifest stays invisible and untouched
// — the default fast path copies BASE into REMOTE, sees no divergence, plans
// nothing, and never repairs. Only --verify looks.
func TestEngine_Run_DefaultPathDoesNotRepairPoison(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	// Flagship poison: disk and manifest both hold A, the server holds B.
	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": contentA,
	}, map[string][]byte{"app.py": []byte(contentB)}))

	manifestPath, _ := statePaths(dir)
	before := readStateFile(t, manifestPath)

	e := engineFor(t, dir, Options{}, fake, catalogID, versionID)

	_, err := e.Run()
	require.NoError(t, err)

	assert.Zero(t, fake.AllFilesCalls(), "the default path must not fetch the remote")
	assert.Equal(t, before, readStateFile(t, manifestPath),
		"without --verify the poison survives the run (the documented residual risk)")
}

// TestEngine_Run_VerifyPersistsNoVerifyState: a --verify run must leave
// exactly the state a plain sync would leave. Two identical projects, one
// synced plainly and one synced with --verify first, end with byte-identical
// manifest.json and config.json — no verify-specific field anywhere, and no
// verify-dependent residue in the next plain sync's state.
func TestEngine_Run_VerifyPersistsNoVerifyState(t *testing.T) {
	const (
		catalogID  = "cid-1"
		versionID  = "ver-1"
		newVersion = "ver-2"
	)

	files := map[string]string{"app.py": "print('A')\n"}

	// A fixed clock makes syncedAt deterministic, so byte-equality of the
	// two projects' state files means the content is identical, not merely
	// shaped the same.
	fixedNow := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	// Plain sync only. The server seed is captured while the tree still
	// holds the pre-change content, then disk moves on.
	dirPlain := syncedProject(t, files, catalogID, versionID)

	plainSeed := seededServerContents(t, dirPlain, files, nil)
	modifyFile(t, dirPlain, "app.py", "print('B')\n")

	plainFake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: newVersion,
	}).withVersionContent(catalogID, versionID, plainSeed)

	ePlain, err := newWithDeps(dirPlain, Options{Yes: true}, Deps{
		Files: plainFake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: func() time.Time { return fixedNow },
	})
	require.NoError(t, err)

	_, err = ePlain.Run()
	require.NoError(t, err)
	require.NoError(t, ePlain.Close())

	// --verify first, then a plain sync on top. Same seed discipline: the
	// server holds the pre-change content, so BASE still describes it and
	// the only divergence in play is the ordinary local modification.
	dirVerify := syncedProject(t, files, catalogID, versionID)

	verifySeed := seededServerContents(t, dirVerify, files, nil)
	modifyFile(t, dirVerify, "app.py", "print('B')\n")

	verifyFake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: newVersion,
	}).withVersionContent(catalogID, versionID, verifySeed)

	eVerify, err := newWithDeps(dirVerify, Options{Yes: true, Verify: true}, Deps{
		Files: verifyFake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: func() time.Time { return fixedNow },
	})
	require.NoError(t, err)

	_, err = eVerify.Run()
	require.NoError(t, err)
	require.NoError(t, eVerify.Close())

	assert.Empty(t, eVerify.Divergences(), "the server holds the pre-change content, so the verify run must find no divergence")

	ePlain2, err := newWithDeps(dirVerify, Options{Yes: true}, Deps{
		Files: verifyFake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, newVersion), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: func() time.Time { return fixedNow },
	})
	require.NoError(t, err)

	_, err = ePlain2.Run()
	require.NoError(t, err)
	require.NoError(t, ePlain2.Close())

	manifestPlainPath, configPlainPath := statePaths(dirPlain)
	manifestVerifyPath, configVerifyPath := statePaths(dirVerify)

	assert.Equal(t, string(readStateFile(t, manifestPlainPath)), string(readStateFile(t, manifestVerifyPath)),
		"the verify run's persisted manifest must be byte-identical to a plain sync's")

	// createdAt is written at init from the real clock, so it legitimately
	// differs between the two projects; compare everything else.
	stripCreatedAt := func(t *testing.T, path string) string {
		t.Helper()

		var cfg map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(readStateFile(t, path), &cfg))

		delete(cfg, "createdAt")

		b, err := json.Marshal(cfg)
		require.NoError(t, err)

		return string(b)
	}

	assert.Equal(t, stripCreatedAt(t, configPlainPath), stripCreatedAt(t, configVerifyPath),
		"the verify run's persisted config must match a plain sync's (modulo init-time createdAt)")

	// Schema discipline: the manifest carries only the four documented keys.
	rawManifest := readStateFile(t, manifestVerifyPath)

	var manifestKeys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rawManifest, &manifestKeys))

	wantKeys := []string{"version", "syncedAt", "syncedVersionId", "files"}
	require.Len(t, manifestKeys, len(wantKeys))

	for _, k := range wantKeys {
		assert.Contains(t, manifestKeys, k)
	}

	// config.json must have grown no verify-related field.
	var configKeys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(readStateFile(t, configVerifyPath), &configKeys))

	for _, forbidden := range []string{"verified", "lastVerifiedVersionId", "lastVerifyAt"} {
		assert.NotContains(t, configKeys, forbidden)
	}
}
