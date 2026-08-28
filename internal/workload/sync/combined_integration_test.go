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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests prove the three fixes compose: hash-while-streaming, --verify
// divergence detection and repair, and symlink surfacing all fire in a
// single run without interfering. Each fix is individually covered by its
// own test file; these are the integration cases that exercise multiple
// fixes at once, driven through Plan/Execute against the self-consistent
// fake.
//
// Fulfills VAL-CROSS-004, VAL-CROSS-006, VAL-CROSS-007, VAL-CROSS-008,
// VAL-CROSS-009, VAL-CROSS-010, VAL-CROSS-012 at the go test level.

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// addFileSymlink creates a file symlink (linkTo -> target) in dir, skipping
// on Windows where os.Symlink needs Developer Mode.
func addFileSymlink(t *testing.T, dir, target, linkTo string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, target), []byte("target content\n"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, target),
		filepath.Join(dir, linkTo)))
}

// addDirSymlink creates a directory symlink (linkTo -> targetDir) in dir,
// with a child file inside the target so the subtree is non-empty.
func addDirSymlink(t *testing.T, dir, targetDir, child, linkTo string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, targetDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, targetDir, child), []byte("child content\n"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, targetDir),
		filepath.Join(dir, linkTo)))
}

// manyFiles builds a map of n small files (file_00.py … file_NN-1.py) for
// tests that need to cross the zip-path threshold (>20 uploads).
func manyFiles(n int) map[string]string {
	out := make(map[string]string, n)
	for i := 0; i < n; i++ {
		out[fmt.Sprintf("file_%02d.py", i)] = fmt.Sprintf("content %d\n", i)
	}

	return out
}

// sha256HexOf computes the SHA-256 hex of a string, matching what the fake
// records and what hashEntries produces.
func sha256HexOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// manifestHashes returns the map of path→hash from a loaded manifest, for
// easy comparison against the fake's AllFiles.
func manifestHashes(t *testing.T, dir string) map[string]string {
	t.Helper()

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	out := make(map[string]string, len(m.Files))
	for p, fm := range m.Files {
		out[p] = fm.Hash
	}

	return out
}

// serverHashes returns the map of path→hash from the fake's AllFiles, for
// comparison against the manifest.
func serverHashes(t *testing.T, fake *fakeFilesClient, catalogID, versionID string) map[string]string {
	t.Helper()

	files, err := fake.AllFiles(catalogID, versionID)
	require.NoError(t, err)

	out := make(map[string]string, len(files))
	for p, fm := range files {
		out[p] = fm.Hash
	}

	return out
}

// ---------------------------------------------------------------------------
// VAL-CROSS-004(a): Symlink + mid-stream rewrite
// ---------------------------------------------------------------------------

// TestCombined_SymlinkAndMidStreamRewrite proves that a file symlink and a
// regular file rewritten mid-stream both fire in one run: the symlink notice
// appears, the symlink is absent from uploads, and the raced file's manifest
// entry holds the streamed hash (not the Phase-2 planned hash).
func TestCombined_SymlinkAndMidStreamRewrite(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x4a"
		versionID = "ver-x4a-synced"
		newVer    = "ver-x4a-new"
	)

	original := "content original\n"
	planContent := "content plantime\n"   // same length as original
	streamContent := "content streamed\n" // same length, different content

	require.Len(t, planContent, len(original), "fixture: same-size rewrite")
	require.Len(t, streamContent, len(original), "fixture: same-size rewrite")

	// Synced project: BASE == LOCAL == REMOTE for app.py.
	dir := syncedProject(t, map[string]string{"app.py": original}, catalogID, versionID)

	// Add a file symlink so both fixes fire in the same run.
	addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

	// Modify app.py so it appears as LOCAL_MODIFIED (needs upload).
	modifyFile(t, dir, "app.py", planContent)

	// Seed the server with the ORIGINAL content (before the disk edit below),
	// so BASE == REMOTE and the plan has an upload row for app.py. The seed
	// is built explicitly because seededServerContents reads from disk, and
	// app.py has already been modified to planContent.
	seed := map[string][]byte{
		ignore.FileName: diskBytes(t, dir, ignore.FileName),
		"app.py":        []byte(original),
		"realfile.py":   []byte("target content\n"),
	}

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-x4a",
		versionID: newVer,
	}).withVersionContent(catalogID, versionID, seed)

	// Add realfile.py to the manifest so BASE == REMOTE for it (it was added
	// after syncedProject, so the manifest doesn't have it yet).
	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	rh, _, err := hashLocal(t, dir, "realfile.py")
	require.NoError(t, err)

	m.Files["realfile.py"] = wapi.FileMeta{Hash: rh, Size: int64(len("target content\n"))}
	require.NoError(t, wapi.SaveManifest(dir, m))

	var logged string

	var execErr error

	var plan *SyncPlan

	// Plan: walks, finds the symlink, hashes app.py with planContent.
	logged = captureWarnLog(t, func() {
		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		plan, err = e.Plan()
		require.NoError(t, err)

		// Between Plan and Execute: rewrite app.py to different bytes of the
		// SAME size. The streamed hash will differ from the Phase-2 planned
		// hash — this is the TOCTOU the upload-integrity fix closes.
		modifyFile(t, dir, "app.py", streamContent)

		_, execErr = e.Execute(plan)
	})

	require.NoError(t, execErr, "the sync must succeed despite the mid-stream rewrite")

	// The symlink notice fires.
	assert.Contains(t, logged, "link_to_file.py",
		"the symlink notice must fire in the same run as the mid-stream rewrite")
	assert.Contains(t, logged, "was not uploaded",
		"the symlink notice must say the symlink was not uploaded")

	// The symlink is absent from the upload plan.
	for _, fa := range plan.Uploads {
		assert.NotEqual(t, "link_to_file.py", fa.Path,
			"the symlink must not appear in the upload list")
	}

	// The raced file's manifest entry holds the streamed hash, not the
	// Phase-2 planned hash.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	streamedHash := sha256HexOf(streamContent)
	planHash := sha256HexOf(planContent)

	assert.Equal(t, streamedHash, manifest.Files["app.py"].Hash,
		"the manifest must hold the streamed hash, not the Phase-2 planned hash")
	assert.NotEqual(t, planHash, manifest.Files["app.py"].Hash,
		"the Phase-2 planned hash must not survive the run")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-004(b): Symlink + poisoned BASE + --verify --dry-run
// ---------------------------------------------------------------------------

// TestCombined_SymlinkAndPoisonedBase_VerifyDryRun proves that a --verify
// --dry-run run with both a symlink and a poisoned BASE produces both the
// skipped-symlink field and the divergence field on the engine, with both
// notices reaching the warn log (stderr). The JSON purity of stdout is
// asserted at the cmd level (TestCmd_Combined_JSON_SymlinkAndDivergence).
func TestCombined_SymlinkAndPoisonedBase_VerifyDryRun(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x4b"
		versionID = "ver-x4b-synced"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	// Synced project: disk and manifest both hold A.
	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	// Add a file symlink.
	addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

	// Poison the manifest: BASE says A but the server holds B.
	poisonManifestHash(t, dir, "app.py", sha256HexOf(contentA))
	// Re-poison to a truly wrong hash (not the disk hash) so the divergence
	// is real: BASE = A_hash, REMOTE = B_hash, and A_hash != B_hash.
	// Actually syncedProject already set the manifest to A's hash. We need
	// the server to hold B while the manifest says A. So poison to a
	// different hash than B.
	poisonedHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	poisonManifestHash(t, dir, "app.py", poisonedHash)

	// Seed the server with B (different from the poisoned BASE).
	seed := seededServerContents(t, dir, map[string]string{"app.py": contentA}, map[string][]byte{
		"app.py": []byte(contentB),
	})

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	var logged string

	e := engineFor(t, dir, Options{Verify: true, DryRun: true, Yes: true}, fake, catalogID, versionID)

	logged = captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	// Both the symlink notice and the divergence notice fire.
	assert.Contains(t, logged, "link_to_file.py",
		"the symlink notice must fire alongside the divergence notice")
	assert.Contains(t, logged, "was not uploaded",
		"the symlink notice must say the symlink was not uploaded")
	assert.Contains(t, logged, "divergence",
		"the divergence notice must fire alongside the symlink notice")
	assert.Contains(t, logged, "app.py",
		"the divergence notice must name the divergent path")

	// Both structured fields are populated on the engine.
	require.Len(t, e.SkippedSymlinks(), 1, "the skipped-symlink field must be populated")
	assert.Equal(t, "link_to_file.py", e.SkippedSymlinks()[0].Path)
	assert.False(t, e.SkippedSymlinks()[0].IsDir)

	require.Len(t, e.Divergences(), 1, "the divergence field must be populated")
	assert.Equal(t, "app.py", e.Divergences()[0].Path)
	assert.Equal(t, DivergenceHashMismatch, e.Divergences()[0].Kind)
}

// ---------------------------------------------------------------------------
// VAL-CROSS-006: Symlink-replacement delete is NOT divergence
// ---------------------------------------------------------------------------

// TestCombined_SymlinkReplacementDelete_NotDivergence proves that a path
// which was a real synced file and is now a symlink produces a delete row
// and a symlink notice, but is NOT reported as BASE-vs-REMOTE divergence.
// A false positive there would train users to ignore the notice.
func TestCombined_SymlinkReplacementDelete_NotDivergence(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x6"
		versionID = "ver-x6-synced"
	)

	// Synced project with a real file that will be replaced by a symlink.
	dir := syncedProject(t, map[string]string{
		"app.py":     "print('hi')\n",
		"oldfile.py": "old content\n",
	}, catalogID, versionID)

	// Seed the server with the original content (BASE == REMOTE).
	seed := seededServerContents(t, dir, map[string]string{
		"app.py":     "print('hi')\n",
		"oldfile.py": "old content\n",
	}, nil)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	// Replace oldfile.py with a symlink. The walk will not follow it, so
	// LOCAL is absent for oldfile.py. BASE and REMOTE still agree (both
	// have the original hash), so this is a LOCAL_DELETED, not a divergence.
	require.NoError(t, os.Remove(filepath.Join(dir, "oldfile.py")))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "app.py"),
		filepath.Join(dir, "oldfile.py")))

	var logged string

	e := engineFor(t, dir, Options{Verify: true, DryRun: true, Yes: true}, fake, catalogID, versionID)

	logged = captureWarnLog(t, func() {
		plan, err := e.Plan()
		require.NoError(t, err)

		// The delete row is present.
		deletePaths := make([]string, 0, len(plan.Deletes))
		for _, fa := range plan.Deletes {
			deletePaths = append(deletePaths, fa.Path)
		}

		assert.Contains(t, deletePaths, "oldfile.py",
			"a path replaced by a symlink must produce a delete row")
	})

	// The symlink notice is present.
	assert.Contains(t, logged, "oldfile.py",
		"the symlink notice must name the replaced path")
	assert.Contains(t, logged, "was not uploaded",
		"the symlink notice must say the symlink was not uploaded")

	// The path is NOT reported as divergence. BASE and REMOTE agree (both
	// have the original hash), so there is no BASE-vs-REMOTE divergence.
	for _, d := range e.Divergences() {
		assert.NotEqual(t, "oldfile.py", d.Path,
			"a symlink-replacement delete must NOT be reported as divergence (false positive)")
	}

	// The engine's skippedSymlinks includes the replaced path.
	assert.Contains(t, symlinkPaths(e.skippedSymlinks), "oldfile.py")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-007: Zip path + divergence + both symlink kinds
// ---------------------------------------------------------------------------

// TestCombined_ZipPathWithDivergenceAndSymlinks proves that a 21+ file zip-path
// project with a divergence and both symlink kinds: every uploaded file's
// manifest hash equals the server checksum, the divergent file's entry
// reflects the true server state after apply, both symlinks are reported with
// correct kinds, and neither enters the archive.
func TestCombined_ZipPathWithDivergenceAndSymlinks(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x7"
		versionID = "ver-x7-synced"
		newVer    = "ver-x7-new"
	)

	// 22 files: 21 will be modified (forcing the zip path with >20 uploads),
	// and the 22nd will have its manifest poisoned (divergence).
	files := manyFiles(22)
	dir := syncedProject(t, files, catalogID, versionID)

	// Add both symlink kinds.
	addFileSymlink(t, dir, "realfile.py", "link_to_file.py")
	addDirSymlink(t, dir, "realdir", "inner.py", "link_to_dir")

	// Seed the server with the original content (BASE == REMOTE before
	// poisoning). Include .drignore so it doesn't show up as a divergence.
	seed := seededServerContents(t, dir, files, nil)

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-x7",
		versionID: newVer,
	}).withVersionContent(catalogID, versionID, seed)

	// Modify 21 files (file_00 through file_20) so there are 21 uploads,
	// forcing the zip path (>20 threshold).
	for i := 0; i < 21; i++ {
		rel := fmt.Sprintf("file_%02d.py", i)
		modifyFile(t, dir, rel, fmt.Sprintf("modified %d\n", i))
	}

	// Poison file_21's manifest hash (divergence on a separate path that
	// is NOT being uploaded — disk and server agree, so CONVERGED → skip).
	poisonedHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	poisonManifestHash(t, dir, "file_21.py", poisonedHash)

	var logged string

	var execErr error

	e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

	logged = captureWarnLog(t, func() {
		_, execErr = e.Run()
	})

	require.NoError(t, execErr, "the zip-path sync with divergence must succeed (exit 0)")

	// Both symlink notices and the divergence notice fire in the warn log.
	assert.Contains(t, logged, "link_to_file.py", "file symlink notice must fire in the zip-path run")
	assert.Contains(t, logged, "link_to_dir", "directory symlink notice must fire in the zip-path run")
	assert.Contains(t, logged, "divergence", "divergence notice must fire in the zip-path run")
	assert.Contains(t, logged, "file_21.py", "divergence must name the poisoned path")

	// Both symlinks are reported with correct kinds.
	skipped := e.SkippedSymlinks()
	assert.Contains(t, symlinkPaths(skipped), "link_to_file.py")
	assert.False(t, symlinkIsDir(skipped, "link_to_file.py"),
		"file symlink must report isDir false")
	assert.Contains(t, symlinkPaths(skipped), "link_to_dir")
	assert.True(t, symlinkIsDir(skipped, "link_to_dir"),
		"directory symlink must report isDir true")

	// The divergence is detected for file_21.py.
	foundDiv := false

	for _, d := range e.Divergences() {
		if d.Path == "file_21.py" {
			foundDiv = true

			assert.Equal(t, DivergenceHashMismatch, d.Kind)
		}
	}

	assert.True(t, foundDiv, "the divergence must name file_21.py")

	// Neither symlink enters the archive (not in the server's AllFiles).
	server, err := fake.AllFiles(catalogID, newVer)
	require.NoError(t, err)

	_, hasFileSymlink := server["link_to_file.py"]
	_, hasDirSymlink := server["link_to_dir"]

	assert.False(t, hasFileSymlink, "the file symlink must not enter the archive")
	assert.False(t, hasDirSymlink, "the directory symlink must not enter the archive")

	// Every uploaded file's manifest hash equals the server checksum.
	manifest := manifestHashes(t, dir)

	for i := 0; i < 21; i++ {
		rel := fmt.Sprintf("file_%02d.py", i)
		assert.Equal(t, server[rel].Hash, manifest[rel],
			"uploaded file %s: manifest hash must equal server checksum", rel)
	}

	// The divergent file's entry reflects the true server state after apply
	// (the poisoned hash is gone; the manifest holds the real remote hash).
	assert.NotEqual(t, poisonedHash, manifest["file_21.py"],
		"the poisoned hash must not survive the run")
	assert.Equal(t, server["file_21.py"].Hash, manifest["file_21.py"],
		"the divergent file's manifest entry must reflect the true server state")

	// The zip path was actually taken (not the stage path).
	assert.Positive(t, fake.UploadFromZipCalls(),
		"the zip path must be taken for >20 uploads")
	assert.Zero(t, fake.UploadToStageCalls(),
		"the stage path must not be used when the zip path is taken")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-008: Maximal mixed plan under --verify
// ---------------------------------------------------------------------------

// TestCombined_MaximalMixedPlan_Verify proves that a single --verify --yes run
// with an upload, a download, a delete, a conflict, a file symlink, a
// directory symlink, and a divergence on a separate path: every plan field
// is populated, every notice reaches stderr, the manifest matches the server
// for every path, and the run exits 0.
//
// The setup uses a poisoned BASE to create the download, conflict, and
// divergence simultaneously on a non-drifted artifact (the server holds the
// same version BASE claims to describe, but BASE is wrong for some paths).
func TestCombined_MaximalMixedPlan_Verify(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x8"
		versionID = "ver-x8-synced"
		newVer    = "ver-x8-new"
	)

	// All files start synced (BASE == LOCAL == REMOTE).
	files := map[string]string{
		"upload_me.py":   "upload original\n",
		"download_me.py": "download original\n",
		"delete_me.py":   "delete original\n",
		"conflict_me.py": "conflict original\n",
		"diverge_me.py":  "diverge original\n",
	}

	dir := syncedProject(t, files, catalogID, versionID)

	// Add both symlink kinds.
	addFileSymlink(t, dir, "realfile.py", "link_to_file.py")
	addDirSymlink(t, dir, "realdir", "inner.py", "link_to_dir")

	// Seed the server with the original content (the real REMOTE).
	seed := seededServerContents(t, dir, files, nil)

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-x8",
		versionID: newVer,
	}).withVersionContent(catalogID, versionID, seed)

	// --- Create each plan shape ---

	// Upload: modify upload_me.py locally. BASE == REMOTE (no divergence
	// for this path), LOCAL differs → LOCAL_MODIFIED → upload.
	modifyFile(t, dir, "upload_me.py", "upload modified\n")

	// Download: rewrite download_me.py to content A, poison BASE to A's
	// hash. Now BASE = A, LOCAL = A, REMOTE = original → REMOTE_MODIFIED →
	// download. Divergence: BASE = A != REMOTE = original.
	downloadA := "download rewritten A\n"
	modifyFile(t, dir, "download_me.py", downloadA)
	poisonManifestHash(t, dir, "download_me.py", sha256HexOf(downloadA))

	// Delete: remove delete_me.py locally. BASE == REMOTE (no divergence),
	// LOCAL absent → LOCAL_DELETED → delete.
	require.NoError(t, os.Remove(filepath.Join(dir, "delete_me.py")))

	// Conflict: rewrite conflict_me.py to content C locally, poison BASE to
	// hash A (different from both original and C). Now BASE = A, LOCAL = C,
	// REMOTE = original → CONFLICT. Divergence: BASE = A != REMOTE = original.
	conflictC := "conflict rewritten C\n"
	modifyFile(t, dir, "conflict_me.py", conflictC)

	poisonedA := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	poisonManifestHash(t, dir, "conflict_me.py", poisonedA)

	// Divergence-only: poison diverge_me.py's manifest to hash A. Disk and
	// server both hold the original, so CONVERGED → skip. Divergence: BASE
	// = A != REMOTE = original.
	poisonManifestHash(t, dir, "diverge_me.py", poisonedA)

	var logged string

	var execErr error

	e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

	logged = captureWarnLog(t, func() {
		_, execErr = e.Run()
	})

	require.NoError(t, execErr, "the maximal mixed plan must succeed (exit 0)")

	// --- Every plan field is populated ---

	plan := e.plan
	require.NotNil(t, plan)
	assert.NotEmpty(t, plan.Uploads, "upload field must be populated")
	assert.NotEmpty(t, plan.Downloads, "download field must be populated")
	assert.NotEmpty(t, plan.Deletes, "delete field must be populated")
	assert.NotEmpty(t, plan.Conflicts, "conflict field must be populated")

	// Verify each action type is present.
	uploadPaths := uploadPathsOf(plan)
	assert.Contains(t, uploadPaths, "upload_me.py", "upload_me.py must be in uploads")

	downloadPaths := make([]string, 0, len(plan.Downloads))
	for _, fa := range plan.Downloads {
		downloadPaths = append(downloadPaths, fa.Path)
	}

	assert.Contains(t, downloadPaths, "download_me.py", "download_me.py must be in downloads")

	deletePaths := make([]string, 0, len(plan.Deletes))
	for _, fa := range plan.Deletes {
		deletePaths = append(deletePaths, fa.Path)
	}

	assert.Contains(t, deletePaths, "delete_me.py", "delete_me.py must be in deletes")

	conflictPaths := plan.ConflictPaths()
	assert.Contains(t, conflictPaths, "conflict_me.py", "conflict_me.py must be in conflicts")

	// --- Every notice reaches stderr (warn log) ---

	// Symlink notices.
	assert.Contains(t, logged, "link_to_file.py", "file symlink notice must reach stderr")
	assert.Contains(t, logged, "link_to_dir", "directory symlink notice must reach stderr")
	assert.Contains(t, logged, "was not uploaded", "file symlink wording must be present")
	assert.Contains(t, logged, "was not synced", "directory symlink wording must be present")

	// Divergence notices.
	assert.Contains(t, logged, "divergence", "divergence notice must reach stderr")
	assert.Contains(t, logged, "download_me.py", "divergence must name download_me.py")
	assert.Contains(t, logged, "conflict_me.py", "divergence must name conflict_me.py")
	assert.Contains(t, logged, "diverge_me.py", "divergence must name diverge_me.py")

	// --- Both symlink kinds are reported ---

	skipped := e.SkippedSymlinks()
	assert.Contains(t, symlinkPaths(skipped), "link_to_file.py")
	assert.False(t, symlinkIsDir(skipped, "link_to_file.py"))
	assert.Contains(t, symlinkPaths(skipped), "link_to_dir")
	assert.True(t, symlinkIsDir(skipped, "link_to_dir"))

	// --- The manifest matches the server for every path ---

	manifest := manifestHashes(t, dir)
	server := serverHashes(t, fake, catalogID, newVer)

	// The uploaded file has the streamed hash.
	assert.Equal(t, server["upload_me.py"], manifest["upload_me.py"],
		"uploaded file: manifest must match server")

	// The downloaded file has the remote hash (server's original).
	assert.Equal(t, server["download_me.py"], manifest["download_me.py"],
		"downloaded file: manifest must match server")

	// The conflict file has the remote hash (remote wins).
	assert.Equal(t, server["conflict_me.py"], manifest["conflict_me.py"],
		"conflict file: manifest must match server (remote wins)")

	// The divergent-only file has the real remote hash (repaired).
	assert.Equal(t, server["diverge_me.py"], manifest["diverge_me.py"],
		"divergent file: manifest must match server (repaired)")

	// The deleted file is absent from both.
	_, manifestHasDelete := manifest["delete_me.py"]
	assert.False(t, manifestHasDelete, "deleted file must not be in the manifest")

	_, serverHasDelete := server["delete_me.py"]
	assert.False(t, serverHasDelete, "deleted file must not be on the server")

	// Neither symlink is in the manifest or server.
	_, manifestHasFileLink := manifest["link_to_file.py"]
	assert.False(t, manifestHasFileLink, "file symlink must not be in the manifest")

	_, manifestHasDirLink := manifest["link_to_dir"]
	assert.False(t, manifestHasDirLink, "directory symlink must not be in the manifest")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-009: All notices at once (engine level)
// ---------------------------------------------------------------------------

// TestCombined_AllNoticesAtOnce proves that the .wapiignore shadow warning,
// symlink notices, and the divergence notice all fire in a single --verify
// --dry-run run without suppressing each other.
//
// The state-migration notice requires a non-preview run (phase0Preflight
// skips migration for preview modes), so it cannot coexist with --dry-run.
// The cmd-level test (TestCmd_Combined_AllNotices_JSONMode) uses the
// fakeEngine to verify the display layer renders all four notice types
// simultaneously, including the migration notice.
func TestCombined_AllNoticesAtOnce(t *testing.T) {
	skipNonWindowsSymlink(t)

	const (
		catalogID = "cid-x9"
		versionID = "ver-x9-synced"
	)

	contentA := "print('A')\n"
	contentB := "print('B')\n"

	// Synced project with app.py.
	dir := syncedProject(t, map[string]string{"app.py": contentA}, catalogID, versionID)

	// Add a file symlink and a directory symlink.
	addFileSymlink(t, dir, "realfile.py", "link_to_file.py")
	addDirSymlink(t, dir, "realdir", "inner.py", "link_to_dir")

	// Add a .wapiignore file alongside .drignore to trigger the shadow warning.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ignore.LegacyFileName), []byte("*.old\n"), 0o644))

	// Poison the manifest so --verify detects divergence.
	poisonedHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	poisonManifestHash(t, dir, "app.py", poisonedHash)

	// Seed the server with B (different from the poisoned BASE).
	seed := seededServerContents(t, dir, map[string]string{"app.py": contentA}, map[string][]byte{
		"app.py": []byte(contentB),
	})

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	var logged string

	e := engineFor(t, dir, Options{Verify: true, DryRun: true, Yes: true}, fake, catalogID, versionID)

	logged = captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	// No notice suppresses another: all four are present in the warn log.

	// Shadow warning.
	assert.Contains(t, logged, wantShadowWarning,
		"the .wapiignore shadow warning must fire alongside the other notices")

	// Symlink notices.
	assert.Contains(t, logged, "link_to_file.py",
		"the file symlink notice must fire alongside the other notices")
	assert.Contains(t, logged, "link_to_dir",
		"the directory symlink notice must fire alongside the other notices")

	// Divergence notice.
	assert.Contains(t, logged, "divergence",
		"the divergence notice must fire alongside the other notices")
	assert.Contains(t, logged, "app.py",
		"the divergence notice must name the divergent path")

	// All four notice types are independently present (none suppresses
	// another). Verify each is a distinct substring.
	assert.Contains(t, logged, wantShadowWarning,
		"shadow warning must be present")
	assert.Contains(t, logged, "link_to_file.py",
		"file symlink notice must be present")
	assert.Contains(t, logged, "link_to_dir",
		"directory symlink notice must be present")
	assert.Contains(t, logged, "divergence",
		"divergence notice must be present")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-010: Default no-verify user experience
// ---------------------------------------------------------------------------

// TestCombined_DefaultNoVerifyUX proves that a plain run without --verify
// across a sequence of operations: records correct hashes, emits no divergence
// notice, makes no extra AllFiles round-trip on a non-drifted artifact, and
// only prints "Up to date." (empty plan) when disk and server genuinely agree.
//
// Each sub-test uses its own project directory to avoid sync-lock contention
// between engines, since the lock is held until Close and the cleanup runs at
// test end, not between sections.
func TestCombined_DefaultNoVerifyUX(t *testing.T) {
	t.Run("first_sync_correct_hashes_no_divergence", func(t *testing.T) {
		skipNonWindowsSymlink(t)

		dir := initProject(t, map[string]string{
			"app.py":      "print('hi')\n",
			"util.py":     "def u(): pass\n",
			"realfile.py": "target\n",
		})

		addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

		fake := &fakeFilesClient{
			catalogID: "cid-fs", stageID: "stage-1", versionID: "ver-1",
		}

		var logged string

		e := engineFor(t, dir, Options{Yes: true}, fake, "", "")

		logged = captureWarnLog(t, func() {
			_, err := e.Run()
			require.NoError(t, err)
		})

		// The symlink notice fires on first sync.
		assert.Contains(t, logged, "link_to_file.py", "symlink notice must fire on first sync")
		// No divergence notice without --verify.
		assert.NotContains(t, logged, "divergence", "no divergence notice without --verify")

		// Manifest hashes are correct (match disk).
		manifest := manifestHashes(t, dir)
		for _, rel := range []string{"app.py", "util.py", ignore.FileName} {
			h, _, err := hashLocal(t, dir, rel)
			require.NoError(t, err)
			assert.Equal(t, h, manifest[rel], "manifest hash for %s must match disk", rel)
		}

		// The symlink is not in the manifest.
		_, hasLink := manifest["link_to_file.py"]
		assert.False(t, hasLink, "symlink must not be in the manifest")
	})

	t.Run("no_change_resync_up_to_date_no_allfiles", func(t *testing.T) {
		const (
			catalogID = "cid-nc"
			versionID = "ver-nc"
		)

		files := map[string]string{"app.py": "print('hi')\n"}
		dir := syncedProject(t, files, catalogID, versionID)

		seed := seededServerContents(t, dir, files, nil)

		fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		plan, err := e.Plan()
		require.NoError(t, err)
		assert.True(t, plan.IsEmpty(), "no-change re-sync must produce an empty plan (Up to date.)")
		assert.Zero(t, fake.AllFilesCalls(), "no extra AllFiles round-trip on a non-drifted artifact")
	})

	t.Run("mid_sync_edit_records_streamed_hash", func(t *testing.T) {
		const (
			catalogID = "cid-me"
			versionID = "ver-me"
			newVer    = "ver-me-new"
		)

		original := "content AAAA\n"
		planContent := "content BBBB\n"   // same length, different from original
		streamContent := "content CCCC\n" // same length, different from planContent

		require.Len(t, planContent, len(original), "fixture: same-size rewrite")
		require.Len(t, streamContent, len(original), "fixture: same-size rewrite")

		dir := syncedProject(t, map[string]string{"app.py": original}, catalogID, versionID)

		// Seed server with original content (before disk edit).
		seed := map[string][]byte{
			ignore.FileName: diskBytes(t, dir, ignore.FileName),
			"app.py":        []byte(original),
		}

		fake := (&fakeFilesClient{
			catalogID: catalogID, stageID: "stage-me", versionID: newVer,
		}).withVersionContent(catalogID, versionID, seed)

		// Modify app.py BEFORE Plan so the plan has an upload row.
		modifyFile(t, dir, "app.py", planContent)

		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		plan, err := e.Plan()
		require.NoError(t, err)
		require.False(t, plan.IsEmpty(), "the modification must produce a plan")

		// Rewrite app.py to different bytes of the same size between Plan and
		// Execute. The streamed hash will differ from the Phase-2 planned hash.
		modifyFile(t, dir, "app.py", streamContent)

		_, err = e.Execute(plan)
		require.NoError(t, err)

		// The manifest holds the streamed hash, not the Phase-2 planned hash.
		manifest := manifestHashes(t, dir)
		assert.Equal(t, sha256HexOf(streamContent), manifest["app.py"],
			"the manifest must hold the streamed hash after a mid-sync edit")
		assert.NotEqual(t, sha256HexOf(planContent), manifest["app.py"],
			"the Phase-2 planned hash must not survive the run")
	})

	t.Run("symlink_replacement_no_divergence_notice", func(t *testing.T) {
		skipNonWindowsSymlink(t)

		const (
			catalogID = "cid-sr"
			versionID = "ver-sr"
			newVer    = "ver-sr-new"
		)

		dir := syncedProject(t, map[string]string{
			"app.py":  "print('hi')\n",
			"util.py": "def u(): pass\n",
		}, catalogID, versionID)

		// Replace util.py with a symlink.
		require.NoError(t, os.Remove(filepath.Join(dir, "util.py")))
		require.NoError(t, os.Symlink(
			filepath.Join(dir, "app.py"),
			filepath.Join(dir, "util.py")))

		// Seed server with original content (before the replacement).
		seed := map[string][]byte{
			ignore.FileName: diskBytes(t, dir, ignore.FileName),
			"app.py":        []byte("print('hi')\n"),
			"util.py":       []byte("def u(): pass\n"),
		}

		fake := (&fakeFilesClient{
			catalogID: catalogID, stageID: "stage-sr", versionID: newVer,
		}).withVersionContent(catalogID, versionID, seed)

		var logged string

		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		logged = captureWarnLog(t, func() {
			_, err := e.Run()
			require.NoError(t, err)
		})

		// The symlink notice fires for the replaced util.py.
		assert.Contains(t, logged, "util.py", "the replaced path must produce a symlink notice")
		// No divergence notice without --verify.
		assert.NotContains(t, logged, "divergence", "no divergence notice without --verify")

		// The manifest no longer has util.py (it was deleted from the server).
		manifest := manifestHashes(t, dir)
		_, hasUtil := manifest["util.py"]
		assert.False(t, hasUtil, "the deleted path must not be in the manifest")
	})

	t.Run("mixed_plan_no_divergence_notice", func(t *testing.T) {
		skipNonWindowsSymlink(t)

		const (
			catalogID = "cid-mx"
			versionID = "ver-mx-1"
			remoteVer = "ver-mx-remote"
			newVer    = "ver-mx-2"
		)

		dir := syncedProject(t, map[string]string{
			"keep.py":   "keep content\n",
			"modify.py": "original\n",
			"remove.py": "to be deleted\n",
		}, catalogID, versionID)

		// Build the seed for the REMOTE version (which the artifact's codeRef
		// points at). The remote version has the original files plus a new
		// file (remote-added → download). Using a different version than the
		// config's LastSyncedVersionID creates drift, so the engine fetches
		// AllFiles and discovers the new file — without --verify.
		seed := map[string][]byte{
			ignore.FileName: diskBytes(t, dir, ignore.FileName),
			"keep.py":       []byte("keep content\n"),
			"modify.py":     []byte("original\n"),
			"remove.py":     []byte("to be deleted\n"),
			"new_remote.py": []byte("remote added\n"),
		}

		// Add a file symlink.
		addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

		// Modify modify.py locally.
		modifyFile(t, dir, "modify.py", "modified content\n")

		// Remove remove.py locally.
		require.NoError(t, os.Remove(filepath.Join(dir, "remove.py")))

		fake := (&fakeFilesClient{
			catalogID: catalogID, stageID: "stage-mx", versionID: newVer,
		}).withVersionContent(catalogID, remoteVer, seed)

		var logged string

		// The artifact points at remoteVer (drifted from versionID in config).
		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, remoteVer)

		logged = captureWarnLog(t, func() {
			_, err := e.Run()
			require.NoError(t, err)
		})

		// No divergence notice without --verify.
		assert.NotContains(t, logged, "divergence", "no divergence notice without --verify in a mixed plan")

		// The symlink notice fires.
		assert.Contains(t, logged, "link_to_file.py", "symlink notice must fire in a mixed plan")

		// Manifest hashes are correct.
		manifest := manifestHashes(t, dir)
		h, _, err := hashLocal(t, dir, "modify.py")
		require.NoError(t, err)
		assert.Equal(t, h, manifest["modify.py"], "modified file: manifest must match disk")
		assert.Equal(t, sha256HexOf("remote added\n"), manifest["new_remote.py"],
			"downloaded file: manifest must match server")
		_, hasRemove := manifest["remove.py"]
		assert.False(t, hasRemove, "deleted file must not be in the manifest")
	})
}

// ---------------------------------------------------------------------------
// VAL-CROSS-012: Exit-code coherence
// ---------------------------------------------------------------------------

// TestCombined_ExitCodeCoherence proves that exit code 0 means success or
// diagnostics-only findings, and non-zero means a genuine failure. At the
// engine level, "exit code" is whether Run() returns an error.
func TestCombined_ExitCodeCoherence(t *testing.T) {
	t.Run("symlinks_only_exit0", func(t *testing.T) {
		skipNonWindowsSymlink(t)

		dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})
		addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

		fake := &fakeFilesClient{
			catalogID: "cid-ec1", stageID: "stage-1", versionID: "ver-1",
		}

		e := engineFor(t, dir, Options{Yes: true}, fake, "", "")

		_, err := e.Run()
		assert.NoError(t, err, "symlinks only: exit 0 (diagnostic, not failure)")
	})

	t.Run("divergence_only_exit0", func(t *testing.T) {
		const (
			catalogID = "cid-ec2"
			versionID = "ver-ec2"
		)

		dir := syncedProject(t, map[string]string{"app.py": "print('A')\n"}, catalogID, versionID)

		poisonedHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		poisonManifestHash(t, dir, "app.py", poisonedHash)

		seed := seededServerContents(t, dir, map[string]string{"app.py": "print('A')\n"},
			map[string][]byte{"app.py": []byte("print('B')\n")})

		fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

		e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

		_, err := e.Run()
		assert.NoError(t, err, "divergence only: exit 0 (diagnostic, not failure)")
	})

	t.Run("symlinks_and_divergence_exit0", func(t *testing.T) {
		skipNonWindowsSymlink(t)

		const (
			catalogID = "cid-ec3"
			versionID = "ver-ec3"
			newVer    = "ver-ec3-new"
		)

		dir := syncedProject(t, map[string]string{"app.py": "print('A')\n"}, catalogID, versionID)
		addFileSymlink(t, dir, "realfile.py", "link_to_file.py")

		poisonedHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		poisonManifestHash(t, dir, "app.py", poisonedHash)

		// The server holds B; disk holds A; BASE is poisoned. Classify
		// sees all three different → CONFLICT → remote wins → download.
		// The symlink target realfile.py is a new file → LOCAL_ADDED → upload.
		// So the fake needs stageID/versionID for the upload path.
		seed := map[string][]byte{
			ignore.FileName: diskBytes(t, dir, ignore.FileName),
			"app.py":        []byte("print('B')\n"),
		}

		fake := (&fakeFilesClient{
			catalogID: catalogID,
			stageID:   "stage-ec3",
			versionID: newVer,
		}).withVersionContent(catalogID, versionID, seed)

		e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

		_, err := e.Run()
		assert.NoError(t, err, "symlinks + divergence: exit 0 (both are diagnostics)")
	})

	t.Run("healthy_up_to_date_exit0", func(t *testing.T) {
		const (
			catalogID = "cid-ec4"
			versionID = "ver-ec4"
		)

		dir := syncedProject(t, map[string]string{"app.py": "print('hi')\n"}, catalogID, versionID)

		fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID,
			seededServerContents(t, dir, map[string]string{"app.py": "print('hi')\n"}, nil))

		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		_, err := e.Run()
		assert.NoError(t, err, "healthy up-to-date: exit 0")
	})

	t.Run("successful_sync_exit0", func(t *testing.T) {
		dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

		fake := &fakeFilesClient{
			catalogID: "cid-ec5", stageID: "stage-1", versionID: "ver-1",
		}

		e := engineFor(t, dir, Options{Yes: true}, fake, "", "")

		_, err := e.Run()
		assert.NoError(t, err, "successful sync: exit 0")
	})

	t.Run("upload_failure_nonzero", func(t *testing.T) {
		const (
			catalogID = "cid-ec6"
			versionID = "ver-ec6"
			newVer    = "ver-ec6-new"
		)

		dir := syncedProject(t, map[string]string{"app.py": "print('A')\n"}, catalogID, versionID)
		modifyFile(t, dir, "app.py", "print('B')\n")

		seed := seededServerContents(t, dir, map[string]string{"app.py": "print('A')\n"}, nil)

		fake := (&fakeFilesClient{
			catalogID: catalogID, stageID: "stage-1", versionID: newVer,
		}).withVersionContent(catalogID, versionID, seed).withFailNthUpload(1)

		e := engineFor(t, dir, Options{Yes: true}, fake, catalogID, versionID)

		_, err := e.Run()
		require.Error(t, err, "upload failure: non-zero exit")
		assert.Contains(t, err.Error(), "upload")
	})

	t.Run("post_apply_verification_mismatch_nonzero", func(t *testing.T) {
		const (
			catalogID = "cid-ec7"
			versionID = "ver-ec7"
			newVer    = "ver-ec7-new"
		)

		appPy := "print('A')\n"
		untouched := "print('u')\n"

		dir := syncedProject(t, map[string]string{
			"app.py":       appPy,
			"untouched.py": untouched,
		}, catalogID, versionID)

		// Build the seed from explicit original bytes BEFORE modifying disk,
		// so the server holds the pre-change content (matching BASE) and the
		// plan has exactly one upload row for app.py.
		seed := map[string][]byte{
			ignore.FileName: diskBytes(t, dir, ignore.FileName),
			"app.py":        []byte(appPy),
			"untouched.py":  []byte(untouched),
		}

		newBody := "print('B')\n"
		modifyFile(t, dir, "app.py", newBody)

		// The post-apply AllFiles (for the NEW version) returns a wrong
		// checksum for app.py, so post-apply verification catches it.
		fake := (&fakeFilesClient{
			catalogID: catalogID, stageID: "stage-1", versionID: newVer,
		}).withVersionContent(catalogID, versionID, seed).
			withWrongChecksumForVersion(newVer, "app.py", "deadbeef")

		e := engineFor(t, dir, Options{Verify: true, Yes: true}, fake, catalogID, versionID)

		_, err := e.Run()
		require.Error(t, err, "post-apply verification mismatch: non-zero exit")
		assert.Contains(t, err.Error(), "post-apply verification")
	})

	t.Run("case_collision_nonzero", func(t *testing.T) {
		if runtime.GOOS == "darwin" {
			t.Skip("case-collision test needs a case-sensitive filesystem; macOS APFS is case-insensitive by default")
		}

		dir := initProject(t, map[string]string{"app.py": "x"})

		// Create a file whose name differs only in case from app.py.
		// On a case-insensitive filesystem this is the same file, so the
		// test skips. On Linux CI (case-sensitive) it produces a collision.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "APP.py"), []byte("y"), 0o644))

		fake := &fakeFilesClient{
			catalogID: "cid-ec8", stageID: "stage-1", versionID: "ver-1",
		}

		e := engineFor(t, dir, Options{Yes: true}, fake, "", "")

		_, err := e.Run()
		require.Error(t, err, "case collision: non-zero exit")
		assert.Contains(t, err.Error(), "case")
	})

	t.Run("locked_non_dry_run_nonzero", func(t *testing.T) {
		dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

		e := lockedEngine(t, dir, Options{Yes: true})

		_, err := e.Run()
		require.Error(t, err, "locked non-dry-run sync: non-zero exit")
		assert.Contains(t, err.Error(), "locked")
	})
}
