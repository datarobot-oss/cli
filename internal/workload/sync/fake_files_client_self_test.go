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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sha256Hex computes the SHA-256 hex of content, matching what the fake
// records and what the real server returns.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}

// stageUpload uploads files through the stage path and applies, returning the
// ApplyStage response. This is the helper for tests that exercise the fake's
// server-state model without going through the full engine.
func stageUpload(t *testing.T, fake *fakeFilesClient, files map[string][]byte) *filesapi.ApplyStageResp {
	t.Helper()

	stage, err := fake.CreateStage(fake.catalogID)
	require.NoError(t, err)

	for path, data := range files {
		err := fake.UploadToStage(fake.catalogID, stage.StageID, path, int64(len(data)), bytes.NewReader(data))
		require.NoError(t, err)
	}

	resp, err := fake.ApplyStage(fake.catalogID, stage.StageID, filesapi.OverwriteReplace)
	require.NoError(t, err)

	return resp
}

// TestFakeServerState_StagePathRecordsContentAndChecksum verifies that after an
// upload+apply, AllFiles for the new version returns exactly the paths uploaded
// with fileChecksum equal to the SHA-256 hex of the recorded bytes.
func TestFakeServerState_StagePathRecordsContentAndChecksum(t *testing.T) {
	files := map[string][]byte{
		"app.py":          []byte("print('hello')\n"),
		"utils/helper.py": []byte("def help(): pass\n"),
	}

	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	resp := stageUpload(t, fake, files)
	require.Equal(t, "ver-1", resp.CatalogVersionID)

	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	// AllFiles must return exactly the paths uploaded, with correct checksums.
	assert.Len(t, all, len(files))

	for path, data := range files {
		fm, ok := all[path]
		require.True(t, ok, "path %s missing from AllFiles", path)
		assert.Equal(t, sha256Hex(data), fm.Hash, "checksum for %s", path)
		assert.Equal(t, int64(len(data)), fm.Size, "size for %s", path)
	}
}

// TestFakeServerState_StagePathMergeSemantics verifies that ApplyStage merges
// staged files into the prior version (REPLACE merge), not replacing the
// whole catalog. Uploading 1 file over a 3-file version yields a 4-file
// version.
func TestFakeServerState_StagePathMergeSemantics(t *testing.T) {
	// Pre-populate a version with 3 files.
	priorFiles := map[string]filesapi.FileMeta{
		"a.py": {Hash: sha256Hex([]byte("a")), Size: 1},
		"b.py": {Hash: sha256Hex([]byte("b")), Size: 1},
		"c.py": {Hash: sha256Hex([]byte("c")), Size: 1},
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-2",
	}).withVersion("cid-1", "ver-1", priorFiles)

	// Upload 1 new file over the 3-file version.
	resp := stageUpload(t, fake, map[string][]byte{
		"d.py": []byte("d"),
	})

	// numFiles must count ALL files in the resulting version (3 + 1 = 4),
	// not just the ones uploaded (1).
	assert.Equal(t, 4, resp.NumFiles, "numFiles must count all files in the resulting version, not just uploaded")

	all, err := fake.AllFiles(fake.catalogID, "ver-2")
	require.NoError(t, err)

	// The resulting version must contain all 4 files: 3 from the prior
	// version plus 1 newly uploaded.
	assert.Len(t, all, 4)

	for path := range priorFiles {
		assert.Contains(t, all, path, "prior file %s must survive the merge", path)
	}

	assert.Contains(t, all, "d.py", "new file must be in the version")
	assert.Equal(t, sha256Hex([]byte("d")), all["d.py"].Hash)
}

// TestWithVersionCopiesTheSeed pins that withVersion copies the caller's map
// into server state: mutating the map after handing it over must not change
// what the fake serves. The fake used to alias the caller's map directly, so
// a reused or later-mutated seed silently rewrote an already-seeded version
// behind the test's back — the kind of cross-test state leak -shuffle=on is
// meant to surface.
func TestWithVersionCopiesTheSeed(t *testing.T) {
	seed := map[string]filesapi.FileMeta{
		"a.py": {Hash: "hash-a", Size: 1},
	}

	fake := (&fakeFilesClient{}).withVersion("cid-1", "ver-1", seed)

	// Mutate the caller's map after handing it over; server state must not
	// follow.
	seed["a.py"] = filesapi.FileMeta{Hash: "hash-mutated", Size: 99}
	seed["injected.py"] = filesapi.FileMeta{Hash: "hash-injected", Size: 5}

	all, err := fake.AllFiles("cid-1", "ver-1")
	require.NoError(t, err)

	fm, ok := all["a.py"]
	require.True(t, ok, "seeded path must remain present")

	assert.Equal(t, "hash-a", fm.Hash, "a later mutation of the caller's map must not reach server state")
	assert.Equal(t, int64(1), fm.Size, "a later mutation of the caller's map must not reach server state")

	assert.NotContains(t, all, "injected.py",
		"paths added to the caller's map after the call must not appear in server state")
}

// TestFakeServerState_StagePathReplacesExisting verifies that uploading a file
// that already exists in the prior version replaces its content (REPLACE
// semantics for staged paths).
func TestFakeServerState_StagePathReplacesExisting(t *testing.T) {
	priorFiles := map[string]filesapi.FileMeta{
		"app.py": {Hash: sha256Hex([]byte("old")), Size: 3},
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-2",
	}).withVersion("cid-1", "ver-1", priorFiles)

	newContent := []byte("new content here")

	resp := stageUpload(t, fake, map[string][]byte{"app.py": newContent})
	assert.Equal(t, 1, resp.NumFiles, "version still has 1 file after replace")

	all, err := fake.AllFiles(fake.catalogID, "ver-2")
	require.NoError(t, err)

	assert.Equal(t, sha256Hex(newContent), all["app.py"].Hash, "checksum must reflect new content")
	assert.Equal(t, int64(len(newContent)), all["app.py"].Size)
}

// TestFakeServerState_ZipPathRecordsContent verifies that the zip upload path
// extracts the archive and records per-file content with SHA-256 checksums.
func TestFakeServerState_ZipPathRecordsContent(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":  "print('hi')\n",
		"main.py": "if __name__ == '__main__': pass\n",
	})

	// Build a real zip from the project files (same as the production ZipUploader).
	plan := &SyncPlan{
		Uploads: []FileAction{
			{Path: "app.py", LocalSize: 11},
			{Path: "main.py", LocalSize: 34},
		},
	}

	zipPath, _, err := buildZip(dir, plan.Uploads)
	require.NoError(t, err)

	zipData, err := os.ReadFile(zipPath)
	require.NoError(t, err)

	fake := &fakeFilesClient{
		catalogID: "cid-zip",
		versionID: "ver-zip",
	}

	resp, err := fake.UploadFromZipNew("wapi-sync.zip", int64(len(zipData)), bytes.NewReader(zipData))
	require.NoError(t, err)

	assert.Equal(t, "cid-zip", resp.CatalogID)
	assert.Equal(t, "ver-zip", resp.CatalogVersionID)

	all, err := fake.AllFiles(fake.catalogID, "ver-zip")
	require.NoError(t, err)

	// The zip path must record the same content and checksums as the stage path.
	assert.Len(t, all, 2)

	for _, fa := range plan.Uploads {
		fm, ok := all[fa.Path]
		require.True(t, ok, "path %s missing from zip AllFiles", fa.Path)

		// The checksum must match the SHA-256 of the file content on disk.
		data, err := os.ReadFile(dir + "/" + fa.Path)
		require.NoError(t, err)

		assert.Equal(t, sha256Hex(data), fm.Hash, "zip checksum for %s", fa.Path)
		assert.Equal(t, int64(len(data)), fm.Size, "zip size for %s", fa.Path)
	}
}

// TestFakePagination_PageSizeOne verifies that AllFiles with page size 1 and
// two files returns both files (the fake follows the next link). If the fake
// failed to follow next links, only the first file would be returned.
func TestFakePagination_PageSizeOne(t *testing.T) {
	files := map[string][]byte{
		"alpha.py": []byte("a"),
		"beta.py":  []byte("b"),
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withPageSize(1)

	stageUpload(t, fake, files)

	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	// Both files must be returned despite page size 1.
	assert.Len(t, all, 2, "AllFiles must follow next links and return all files")
	assert.Contains(t, all, "alpha.py")
	assert.Contains(t, all, "beta.py")
}

// TestFakePagination_SinglePage verifies that when all files fit in one page,
// AllFiles returns them all without needing to follow a next link.
func TestFakePagination_SinglePage(t *testing.T) {
	files := map[string][]byte{
		"alpha.py": []byte("a"),
		"beta.py":  []byte("b"),
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withPageSize(10) // larger than the number of files

	stageUpload(t, fake, files)

	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	assert.Len(t, all, 2)
}

// TestFakeNumFiles_DefaultsToAllFiles verifies that ApplyStage returns numFiles
// equal to the count of ALL files in the resulting version, not just the ones
// uploaded.
func TestFakeNumFiles_DefaultsToAllFiles(t *testing.T) {
	priorFiles := map[string]filesapi.FileMeta{
		"a.py": {Hash: sha256Hex([]byte("a")), Size: 1},
		"b.py": {Hash: sha256Hex([]byte("b")), Size: 1},
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-2",
	}).withVersion("cid-1", "ver-1", priorFiles)

	// Upload 1 file over a 2-file version.
	resp := stageUpload(t, fake, map[string][]byte{"c.py": []byte("c")})
	assert.Equal(t, 3, resp.NumFiles, "numFiles = all files in version (2+1=3), not uploaded count (1)")
}

// TestFakeNumFiles_Override verifies that the numFilesOverride hook makes
// ApplyStage return a configured value instead of the real count.
func TestFakeNumFiles_Override(t *testing.T) {
	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withNumFilesOverride(99)

	resp := stageUpload(t, fake, map[string][]byte{"app.py": []byte("x")})
	assert.Equal(t, 99, resp.NumFiles, "numFilesOverride must override the real count")
}

// TestFakeCallCounters verifies that per-method call counters are incremented
// correctly for stage-create, upload-to-stage, apply-stage, AllFiles, and
// DownloadFile.
func TestFakeCallCounters(t *testing.T) {
	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	// No calls yet.
	assert.Equal(t, 0, fake.CreateStageCalls())
	assert.Equal(t, 0, fake.UploadToStageCalls())
	assert.Equal(t, 0, fake.ApplyStageCalls())
	assert.Equal(t, 0, fake.AllFilesCalls())
	assert.Equal(t, 0, fake.UploadFromZipCalls())
	assert.Equal(t, 0, fake.DownloadFileCalls())

	stage, err := fake.CreateStage(fake.catalogID)
	require.NoError(t, err)

	assert.Equal(t, 1, fake.CreateStageCalls())

	for i := 0; i < 3; i++ {
		path := "file" + string(rune('a'+i)) + ".py"
		err := fake.UploadToStage(fake.catalogID, stage.StageID, path, 1, strings.NewReader("x"))
		require.NoError(t, err)
	}

	assert.Equal(t, 3, fake.UploadToStageCalls())

	_, err = fake.ApplyStage(fake.catalogID, stage.StageID, filesapi.OverwriteReplace)
	require.NoError(t, err)

	assert.Equal(t, 1, fake.ApplyStageCalls())

	_, err = fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	_, err = fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	assert.Equal(t, 2, fake.AllFilesCalls())

	var buf bytes.Buffer

	_, _, err = fake.DownloadFile(fake.catalogID, "ver-1", "filea.py", &buf)
	require.NoError(t, err)
	assert.Equal(t, "x", buf.String(), "the downloaded bytes must be what was staged")

	assert.Equal(t, 1, fake.DownloadFileCalls())
}

// TestFakeFaultInjection_DropPath verifies that the dropPathFromApply hook
// omits a path from the resulting version. Passes with the fault off, fails
// with it on (the path is missing from AllFiles).
func TestFakeFaultInjection_DropPath(t *testing.T) {
	files := map[string][]byte{
		"app.py":    []byte("app"),
		"config.py": []byte("config"),
	}

	// Without the fault: both files present.
	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	stageUpload(t, fake, files)

	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)
	assert.Len(t, all, 2, "without fault, both files present")

	// With the fault: app.py is dropped from the resulting version.
	fake2 := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-2",
	}).withDropPath("app.py")

	stageUpload(t, fake2, files)

	all2, err := fake2.AllFiles(fake2.catalogID, "ver-2")
	require.NoError(t, err)

	assert.Len(t, all2, 1, "with fault, dropped path is missing")
	assert.NotContains(t, all2, "app.py", "app.py must be dropped")
	assert.Contains(t, all2, "config.py", "other files must survive")
}

// TestFakeFaultInjection_WrongChecksum verifies that the wrongChecksum hook
// makes AllFiles return a wrong checksum for a chosen path. Passes with the
// fault off, fails with it on.
func TestFakeFaultInjection_WrongChecksum(t *testing.T) {
	data := []byte("content here")
	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	stageUpload(t, fake, map[string][]byte{"app.py": data})

	// Without the fault: correct checksum.
	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)
	assert.Equal(t, sha256Hex(data), all["app.py"].Hash)

	// With the fault: wrong checksum.
	wrong := strings.Repeat("z", 64)
	fake2 := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withWrongChecksum("app.py", wrong)

	stageUpload(t, fake2, map[string][]byte{"app.py": data})

	all2, err := fake2.AllFiles(fake2.catalogID, "ver-1")
	require.NoError(t, err)

	assert.Equal(t, wrong, all2["app.py"].Hash, "wrong checksum must be returned")
	assert.NotEqual(t, sha256Hex(data), all2["app.py"].Hash, "wrong checksum must differ from real")
}

// TestFakeFaultInjection_FailNthUpload verifies that the failNthUpload hook
// fails the Nth UploadToStage call. Passes with the fault off, fails with it on.
func TestFakeFaultInjection_FailNthUpload(t *testing.T) {
	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withFailNthUpload(2)

	stage, err := fake.CreateStage(fake.catalogID)
	require.NoError(t, err)

	// First upload succeeds.
	err = fake.UploadToStage(fake.catalogID, stage.StageID, "a.py", 1, strings.NewReader("a"))
	require.NoError(t, err, "first upload must succeed")

	// Second upload fails.
	err = fake.UploadToStage(fake.catalogID, stage.StageID, "b.py", 1, strings.NewReader("b"))
	require.Error(t, err, "second upload must fail with fault injection")
	require.Contains(t, err.Error(), "injected failure")

	// Third upload succeeds (fault only fires on the Nth call).
	err = fake.UploadToStage(fake.catalogID, stage.StageID, "c.py", 1, strings.NewReader("c"))
	require.NoError(t, err, "third upload must succeed after the fault fired")

	assert.Equal(t, 3, fake.UploadToStageCalls())
}

// TestFakeFaultInjection_FailApplyStage verifies that the failApplyStage hook
// makes ApplyStage return an error after all files have been staged.
func TestFakeFaultInjection_FailApplyStage(t *testing.T) {
	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}).withFailApplyStage()

	stage, err := fake.CreateStage(fake.catalogID)
	require.NoError(t, err)

	err = fake.UploadToStage(fake.catalogID, stage.StageID, "app.py", 3, strings.NewReader("app"))
	require.NoError(t, err, "upload must succeed; fault is on ApplyStage only")

	_, err = fake.ApplyStage(fake.catalogID, stage.StageID, filesapi.OverwriteReplace)
	require.Error(t, err, "ApplyStage must fail with fault injection")
	require.Contains(t, err.Error(), "injected failure")
}

// TestFakeRaceFree_ConcurrentUploads verifies that the fake is race-free when
// the uploader runs with 4-way concurrency over 8+ files. Run with -race to
// detect data races.
func TestFakeRaceFree_ConcurrentUploads(t *testing.T) {
	// 8 files, uploaded concurrently by 4 goroutines (matching UploadConcurrency).
	numFiles := 8

	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	stage, err := fake.CreateStage(fake.catalogID)
	require.NoError(t, err)

	var wg sync.WaitGroup

	for i := 0; i < numFiles; i++ {
		path := "file" + string(rune('a'+i)) + ".py"
		data := []byte(strings.Repeat("x", i+1)) // varying sizes including 0-byte-like

		wg.Add(1)

		go func() {
			defer wg.Done()

			err := fake.UploadToStage(fake.catalogID, stage.StageID, path, int64(len(data)), bytes.NewReader(data))
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	resp, err := fake.ApplyStage(fake.catalogID, stage.StageID, filesapi.OverwriteReplace)
	require.NoError(t, err)

	assert.Equal(t, numFiles, resp.NumFiles)
	assert.Equal(t, numFiles, fake.UploadToStageCalls())

	// Verify all files are present with correct checksums.
	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)

	assert.Len(t, all, numFiles, "all files must be recorded despite concurrent uploads")

	// Verify a few checksums deterministically.
	paths := make([]string, 0, numFiles)
	for i := 0; i < numFiles; i++ {
		paths = append(paths, "file"+string(rune('a'+i))+".py")
	}

	sort.Strings(paths)

	for i, p := range paths {
		expected := sha256Hex([]byte(strings.Repeat("x", i+1)))
		assert.Equal(t, expected, all[p].Hash, "checksum for %s", p)
	}
}

// ---------------------------------------------------------------------------
// Download modelling
// ---------------------------------------------------------------------------

// TestFakeDownload_ServesRecordedBytes_StagePath verifies that DownloadFile
// returns byte-for-byte what the stage path recorded for a path at a
// version, so a download test can assert "the right bytes were served"
// rather than merely "no error was returned".
func TestFakeDownload_ServesRecordedBytes_StagePath(t *testing.T) {
	files := map[string][]byte{
		"app.py":          []byte("print('hello')\n"),
		"utils/helper.py": []byte("def help(): pass\n"),
	}

	fake := &fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-1",
	}

	stageUpload(t, fake, files)

	for path, want := range files {
		var buf bytes.Buffer

		_, n, err := fake.DownloadFile(fake.catalogID, "ver-1", path, &buf)
		require.NoError(t, err, "download %s", path)

		assert.Equal(t, string(want), buf.String(), "downloaded bytes for %s", path)
		assert.Equal(t, int64(len(want)), n, "downloaded byte count for %s", path)
	}

	assert.Equal(t, len(files), fake.DownloadFileCalls())
}

// TestFakeDownload_ServesRecordedBytes_ZipPath verifies that the zip upload
// path also records content, so downloads work for zip-synced catalogs too.
func TestFakeDownload_ServesRecordedBytes_ZipPath(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":  "print('hi')\n",
		"main.py": "if __name__ == '__main__': pass\n",
	})

	plan := &SyncPlan{
		Uploads: []FileAction{
			{Path: "app.py", LocalSize: 11},
			{Path: "main.py", LocalSize: 34},
		},
	}

	zipPath, _, err := buildZip(dir, plan.Uploads)
	require.NoError(t, err)

	zipData, err := os.ReadFile(zipPath)
	require.NoError(t, err)

	fake := &fakeFilesClient{
		catalogID: "cid-zip",
		versionID: "ver-zip",
	}

	_, err = fake.UploadFromZipNew("wapi-sync.zip", int64(len(zipData)), bytes.NewReader(zipData))
	require.NoError(t, err)

	for _, fa := range plan.Uploads {
		want, err := os.ReadFile(filepath.Join(dir, fa.Path))
		require.NoError(t, err)

		var buf bytes.Buffer

		_, n, err := fake.DownloadFile(fake.catalogID, "ver-zip", fa.Path, &buf)
		require.NoError(t, err, "download %s", fa.Path)

		assert.Equal(t, string(want), buf.String(), "zip-path downloaded bytes for %s", fa.Path)
		assert.Equal(t, int64(len(want)), n, "zip-path downloaded byte count for %s", fa.Path)
	}
}

// TestFakeDownload_WithVersionContentConsistentWithAllFiles verifies the
// self-consistency the whole fake rests on: a version seeded through
// withVersionContent advertises via AllFiles exactly the checksums its
// recorded bytes hash to, and DownloadFile serves exactly those bytes. A
// fake whose advertised checksums disagree with its served bytes would make
// every checksum-verification test vacuous.
func TestFakeDownload_WithVersionContentConsistentWithAllFiles(t *testing.T) {
	contents := map[string][]byte{
		"app.py": []byte("server holds this\n"),
		"b.py":   []byte("and this"),
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		versionID: "ver-1",
	}).withVersionContent("cid-1", "ver-1", contents)

	all, err := fake.AllFiles(fake.catalogID, "ver-1")
	require.NoError(t, err)
	assert.Len(t, all, len(contents))

	for path, want := range contents {
		fm, ok := all[path]
		require.True(t, ok, "path %s missing from AllFiles", path)

		assert.Equal(t, sha256Hex(want), fm.Hash, "advertised checksum for %s must be the bytes' hash", path)
		assert.Equal(t, int64(len(want)), fm.Size, "advertised size for %s", path)

		var buf bytes.Buffer

		_, _, err := fake.DownloadFile(fake.catalogID, "ver-1", path, &buf)
		require.NoError(t, err, "download %s", path)

		assert.Equal(t, string(want), buf.String(), "served bytes for %s must match the advertised content", path)
	}
}

// TestFakeDownload_UnknownPathOrVersionErrors pins the faithful-404
// behaviour: an unknown version, an unknown path in a known version, and a
// hash-only withVersion seed (metadata without recorded content) all error
// rather than serving empty bytes that would hash like the empty string.
func TestFakeDownload_UnknownPathOrVersionErrors(t *testing.T) {
	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		versionID: "ver-1",
	}).withVersionContent("cid-1", "ver-1", map[string][]byte{"a.py": []byte("a")})

	t.Run("unknown version", func(t *testing.T) {
		var buf bytes.Buffer

		_, _, err := fake.DownloadFile(fake.catalogID, "ver-missing", "a.py", &buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("unknown path in known version", func(t *testing.T) {
		var buf bytes.Buffer

		_, _, err := fake.DownloadFile(fake.catalogID, "ver-1", "missing.py", &buf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in version")
	})

	t.Run("hash-only seed has no recorded content", func(t *testing.T) {
		hashOnly := (&fakeFilesClient{
			catalogID: "cid-2",
			versionID: "ver-2",
		}).withVersion("cid-2", "ver-2", map[string]filesapi.FileMeta{
			"a.py": {Hash: sha256Hex([]byte("a")), Size: 1},
		})

		var buf bytes.Buffer

		_, _, err := hashOnly.DownloadFile("cid-2", "ver-2", "a.py", &buf)
		require.Error(t, err, "a version seeded with hashes only has no bytes to serve")
		assert.Contains(t, err.Error(), "no recorded content")
	})
}

// TestFakeDownload_ContentCarriedAcrossStageMerge verifies that applying a
// stage over an existing version carries the prior version's recorded
// content into the new version, so files the sync did not touch remain
// downloadable — matching the real API's REPLACE-merge semantics.
func TestFakeDownload_ContentCarriedAcrossStageMerge(t *testing.T) {
	prior := map[string][]byte{
		"a.py": []byte("alpha"),
		"b.py": []byte("beta"),
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		stageID:   "stage-1",
		versionID: "ver-2",
	}).withVersionContent("cid-1", "ver-1", prior)

	stageUpload(t, fake, map[string][]byte{"c.py": []byte("gamma")})

	all, err := fake.AllFiles(fake.catalogID, "ver-2")
	require.NoError(t, err)
	assert.Len(t, all, 3, "merge must keep prior paths and add the new one")

	for path, want := range map[string][]byte{
		"a.py": prior["a.py"],
		"b.py": prior["b.py"],
		"c.py": []byte("gamma"),
	} {
		var buf bytes.Buffer

		_, _, err := fake.DownloadFile(fake.catalogID, "ver-2", path, &buf)
		require.NoError(t, err, "download %s from merged version", path)

		assert.Equal(t, string(want), buf.String(), "carried content for %s", path)
	}
}

// TestFakeDownload_ContentCarriedAcrossDelete verifies that a remote delete
// produces a new version whose surviving paths stay downloadable and whose
// deleted path is gone.
func TestFakeDownload_ContentCarriedAcrossDelete(t *testing.T) {
	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		versionID: "ver-1",
	}).withVersionContent("cid-1", "ver-1", map[string][]byte{
		"a.py": []byte("alpha"),
		"b.py": []byte("beta"),
	})

	resp, err := fake.DeleteFiles("cid-1", []string{"a.py"})
	require.NoError(t, err)

	newVer := resp.CatalogVersionID
	require.NotEmpty(t, newVer)

	var buf bytes.Buffer

	_, _, err = fake.DownloadFile("cid-1", newVer, "b.py", &buf)
	require.NoError(t, err, "surviving path must stay downloadable after a delete")
	assert.Equal(t, "beta", buf.String())

	_, _, err = fake.DownloadFile("cid-1", newVer, "a.py", &buf)
	require.Error(t, err, "deleted path must no longer be downloadable")
}

// TestFakeFaultInjection_FailDownload verifies the fail-download hook:
// without it the same call succeeds (control), with it DownloadFile errors
// and the call is still counted.
func TestFakeFaultInjection_FailDownload(t *testing.T) {
	seed := func() *fakeFilesClient {
		return (&fakeFilesClient{
			catalogID: "cid-1",
			versionID: "ver-1",
		}).withVersionContent("cid-1", "ver-1", map[string][]byte{
			"app.py": []byte("content"),
		})
	}

	// Control: no fault, download succeeds.
	plain := seed()

	var buf bytes.Buffer

	_, _, err := plain.DownloadFile("cid-1", "ver-1", "app.py", &buf)
	require.NoError(t, err, "without the fault the download must succeed")
	assert.Equal(t, 1, plain.DownloadFileCalls())

	// With the fault: the download fails and still counts as a call.
	faulty := seed().withFailDownload("app.py")

	_, _, err = faulty.DownloadFile("cid-1", "ver-1", "app.py", &buf)
	require.Error(t, err, "the faulted download must fail")
	assert.Contains(t, err.Error(), "injected failure")
	assert.Equal(t, 1, faulty.DownloadFileCalls(), "a faulted call still counts")
}

// TestFakeFaultInjection_CorruptDownload verifies the corrupt-download
// hook: the served bytes keep their length (so a size check passes) but
// hash to something other than the advertised checksum, which is what lets
// a test target the client-side checksum verification specifically. The
// control pins that the unhooked fake serves bytes matching the advertised
// checksum.
func TestFakeFaultInjection_CorruptDownload(t *testing.T) {
	content := []byte("genuine bytes")

	seed := func() *fakeFilesClient {
		return (&fakeFilesClient{
			catalogID: "cid-1",
			versionID: "ver-1",
		}).withVersionContent("cid-1", "ver-1", map[string][]byte{
			"app.py": content,
		})
	}

	// Control: unhooked fake serves bytes whose hash matches AllFiles'.
	plain := seed()

	var plainBuf bytes.Buffer

	_, _, err := plain.DownloadFile("cid-1", "ver-1", "app.py", &plainBuf)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex(content), sha256Hex(plainBuf.Bytes()),
		"without the fault the served bytes must match the advertised checksum")

	// With the fault: same length, different bytes, different hash.
	faulty := seed().withCorruptDownload("app.py")

	var corruptBuf bytes.Buffer

	_, n, err := faulty.DownloadFile("cid-1", "ver-1", "app.py", &corruptBuf)
	require.NoError(t, err, "the corruption hook alters bytes, not the outcome")

	all, err := faulty.AllFiles("cid-1", "ver-1")
	require.NoError(t, err)

	assert.Equal(t, int64(len(content)), n, "corrupt bytes must keep the advertised size")
	assert.Equal(t, len(content), corruptBuf.Len(), "corrupt bytes must be the same length")
	assert.NotEqual(t, content, corruptBuf.Bytes(), "corrupt bytes must differ from the recorded content")
	assert.NotEqual(t, all["app.py"].Hash, sha256Hex(corruptBuf.Bytes()),
		"corrupt bytes must hash to something other than the advertised checksum")
}

// TestFakeRaceFree_ConcurrentDownloads verifies the download path is
// race-free under the same concurrency the real downloader uses
// (DownloadConcurrency = 4 workers over more files). Run with -race.
func TestFakeRaceFree_ConcurrentDownloads(t *testing.T) {
	const numFiles = 8

	contents := make(map[string][]byte, numFiles)

	for i := 0; i < numFiles; i++ {
		contents["file"+string(rune('a'+i))+".py"] = []byte(strings.Repeat("x", i+1))
	}

	fake := (&fakeFilesClient{
		catalogID: "cid-1",
		versionID: "ver-1",
	}).withVersionContent("cid-1", "ver-1", contents)

	paths := make([]string, 0, numFiles)
	for p := range contents {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	var wg sync.WaitGroup

	for _, path := range paths {
		want := contents[path]

		wg.Add(1)

		go func() {
			defer wg.Done()

			var buf bytes.Buffer

			_, _, err := fake.DownloadFile("cid-1", "ver-1", path, &buf)
			assert.NoError(t, err)
			assert.Equal(t, string(want), buf.String(), "concurrent download bytes for %s", path)
		}()
	}

	wg.Wait()

	assert.Equal(t, numFiles, fake.DownloadFileCalls())
}
