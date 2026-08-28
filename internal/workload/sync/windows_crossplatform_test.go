// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests provide Windows unit coverage requested on the ticket, scoped
// to unit tests only. They run on ALL platforms (including Windows) for the
// platform-independent behaviours, and explicitly skip with a visible
// reason for platform-inappropriate checks.
//
// The Windows CI leg has no -race and is continue-on-error (CFX-7059), so
// Linux remains the only race gate. These tests are written to be
// meaningful even on a best-effort leg: every skip carries a visible reason
// so a CI summary distinguishes "skipped on Windows" from "passed on
// Windows".
//
// Fulfills VAL-SYMLINK-012 and VAL-REGRESSION-013.

// ---------------------------------------------------------------------------
// SHA-256 parity (VAL-REGRESSION-013a)
// ---------------------------------------------------------------------------

// TestSHA256Parity_CRLFContent asserts that a file's computed SHA-256 hash
// is the same regardless of platform, including content containing CRLF
// line endings. Go's os.Open opens files in binary mode by default, so
// \r\n must not be translated to \n. The test guards against any future
// regression that introduces text-mode handling (e.g., a bufio.Scanner
// that strips \r, or a file open with O_TEXT on Windows).
//
// The expected hash is computed directly from the raw bytes via
// crypto/sha256, so the assertion is against a platform-independent
// reference. If any layer translates \r\n to \n (or vice versa), the hash
// will differ and the test will fail.
//
// Fulfills VAL-REGRESSION-013(a).
func TestSHA256Parity_CRLFContent(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
	}{
		// CRLF line endings — the primary case. If \r\n is translated to
		// \n by any layer, the hash changes.
		{name: "CRLFOnly", content: []byte("line1\r\nline2\r\nline3\r\n")},
		// Mixed line endings — CRLF and LF in the same file.
		{name: "MixedLineEndings", content: []byte("unix\nwindows\r\nmixed\r\nunix\n")},
		// LF-only — the control case. Should always match.
		{name: "LFOnly", content: []byte("line1\nline2\nline3\n")},
		// Empty file — the SHA-256 of the empty string is a known constant.
		{name: "EmptyFile", content: []byte{}},
		// Binary data containing \r without \n — must not be stripped.
		{name: "BinaryWithCR", content: []byte("data\r\x00more\rdata")},
		// A lone \r at end of file — must not be stripped or translated.
		{name: "TrailingCR", content: []byte("content\r")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Compute the expected hash directly from the raw bytes.
			expected := sha256.Sum256(tc.content)
			expectedHex := hex.EncodeToString(expected[:])

			// Write the exact bytes to a temp file.
			dir := t.TempDir()
			p := filepath.Join(dir, "testfile.bin")
			require.NoError(t, os.WriteFile(p, tc.content, 0o644))

			gotHash, gotSize, err := fileops.HashFile(p)
			require.NoError(t, err)

			assert.Equal(t, expectedHex, gotHash,
				"SHA-256 must match the raw bytes exactly — no text-mode translation of \\r\\n")
			assert.Equal(t, int64(len(tc.content)), gotSize,
				"byte size must match the raw content length — no byte added or removed")
		})
	}
}

// TestSHA256Parity_StreamedMatchesFileHash asserts that the streaming hash
// used during uploads (newStreamHasher + streamedEntry) produces the same
// SHA-256 as fileops.HashFile for the same bytes, including CRLF content.
// The manifest records the streamed hash; the plan compares against the
// file hash. If they diverged, every file would appear modified on every
// sync — a silent correctness bug.
//
// Fulfills VAL-REGRESSION-013(a) for the streaming hash path.
func TestSHA256Parity_StreamedMatchesFileHash(t *testing.T) {
	cases := [][]byte{
		[]byte("line1\r\nline2\r\nline3\r\n"),
		[]byte("unix\nwindows\r\nmixed\r\n"),
		[]byte("binary\r\x00data\rmore"),
		{},
	}

	for _, content := range cases {
		t.Run("size_"+itoa(len(content)), func(t *testing.T) {
			// File hash via fileops.HashFile.
			dir := t.TempDir()
			p := filepath.Join(dir, "stream.bin")
			require.NoError(t, os.WriteFile(p, content, 0o644))

			fileHash, fileSize, err := fileops.HashFile(p)
			require.NoError(t, err)

			// Streamed hash via the same pipeline the uploaders use.
			h := newStreamHasher()
			_, err = h.Write(content)
			require.NoError(t, err)

			streamed := streamedEntry(h, int64(len(content)))

			assert.Equal(t, fileHash, streamed.Hash,
				"streamed SHA-256 must match file SHA-256 for the same bytes")
			assert.Equal(t, fileSize, streamed.Size,
				"streamed size must match file size for the same bytes")
		})
	}
}

// ---------------------------------------------------------------------------
// Path formatting (VAL-REGRESSION-013b)
// ---------------------------------------------------------------------------

// TestPathFormatting_PlanUsesForwardSlashes asserts that all paths in the
// sync plan use forward slashes, identical to POSIX. On Windows the
// OS-native walker produces backslash-separated paths; NormalizePath
// converts them to forward slashes. This test runs on all platforms —
// trivially passing on POSIX where the separator is already '/', and
// meaningfully on Windows where the conversion is load-bearing.
//
// Fulfills VAL-REGRESSION-013(b) for plan output paths.
func TestPathFormatting_PlanUsesForwardSlashes(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":                "print('hi')\n",
		"sub/dir/helper.py":     "def help(): pass\n",
		"deep/nested/path/f.py": "x\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	require.NotEmpty(t, plan.Uploads, "plan must have uploads to assert path formatting")

	checkForwardSlash := func(t *testing.T, path string, label string) {
		t.Helper()

		assert.NotContains(t, path, "\\", "%s path must use forward slashes, got: %s", label, path)
	}

	for _, fa := range plan.Uploads {
		checkForwardSlash(t, fa.Path, "upload")
	}

	for _, fa := range plan.Downloads {
		checkForwardSlash(t, fa.Path, "download")
	}

	for _, fa := range plan.Deletes {
		checkForwardSlash(t, fa.Path, "delete")
	}

	for _, fa := range plan.Conflicts {
		checkForwardSlash(t, fa.Path, "conflict")
	}
}

// TestPathFormatting_ManifestKeysForwardSlashes asserts that manifest.json
// keys use forward slashes after a sync, identical to POSIX. The manifest
// is written from the plan paths, which are already normalized. This test
// runs on all platforms.
//
// Fulfills VAL-REGRESSION-013(b) for manifest keys.
func TestPathFormatting_ManifestKeysForwardSlashes(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":                "print('hi')\n",
		"sub/dir/helper.py":     "def help(): pass\n",
		"deep/nested/path/f.py": "x\n",
	})

	// The fake must have IDs configured so CreateCatalog and ApplyStage
	// succeed during Execute — a first-sync scenario against an empty
	// artifact.
	fake := &fakeFilesClient{
		catalogID: "cid-paths",
		stageID:   "stage-paths",
		versionID: "ver-paths",
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	require.NotEmpty(t, plan.Uploads, "plan must have uploads to produce a manifest")

	_, err = e.Execute(plan)
	require.NoError(t, err)

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	require.NotEmpty(t, manifest.Files, "manifest must have file entries after sync")

	for key := range manifest.Files {
		assert.NotContains(t, key, "\\",
			"manifest key must use forward slashes, got: %s", key)
	}
}

// ---------------------------------------------------------------------------
// No-symlink project on Windows (VAL-SYMLINK-012)
// ---------------------------------------------------------------------------

// TestNoSymlinkProject_SyncsWithNoNotice asserts that a normal project with
// no symlinks produces no symlink notice and syncs normally. The no-symlink
// code path is platform-independent: the walk simply finds no symlinks to
// report, so skippedSymlinks stays empty and no warning is emitted. This
// test runs on all platforms, including Windows where symlink creation
// needs Developer Mode — a project without symlinks is the common case.
//
// Fulfills VAL-SYMLINK-012.
func TestNoSymlinkProject_SyncsWithNoNotice(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":           "print('hi')\n",
		"sub/helper.py":    "def help(): pass\n",
		"requirements.txt": "flask\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	// Capture the warn log to verify no symlink notice is emitted.
	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err, "a no-symlink project must sync without error")
	})

	// No symlink notice is emitted.
	assert.NotContains(t, logged, "symlink",
		"a project with no symlinks must emit no symlink-related warning")

	// The skippedSymlinks field is empty.
	assert.Empty(t, e.skippedSymlinks,
		"the skippedSymlinks field must be empty when there are no symlinks")

	// The plan contains the real files (the sync works normally).
	uploadPaths := uploadPathsOf(e.plan)
	assert.Contains(t, uploadPaths, "app.py")
	assert.Contains(t, uploadPaths, "sub/helper.py")
	assert.Contains(t, uploadPaths, "requirements.txt")
}

// ---------------------------------------------------------------------------
// Platform-inappropriate check skips
// ---------------------------------------------------------------------------

// TestFileModePreservation_SkipOnWindows documents that file-mode
// assertions are skipped on Windows with a visible reason. Windows
// collapses POSIX mode bits: a file created with 0o644 and one created
// with 0o600 are indistinguishable through os.Stat on NTFS. Any test that
// asserts a specific file mode after a sync would silently pass on
// Windows even if the mode were wrong, so it must skip rather than report
// a vacuous pass.
//
// This test is the explicit skip: it creates a file with a specific mode,
// verifies the mode on POSIX, and skips on Windows with a visible reason.
// It serves as the documented precedent for future file-mode tests.
//
// Fulfills VAL-REGRESSION-013(c) for file-mode assertions.
func TestFileModePreservation_SkipOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-mode assertions skipped on Windows: Windows collapses POSIX mode bits (0o644 vs 0o600 are indistinguishable on NTFS)")
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "mode_test.py")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))

	info, err := os.Stat(p)
	require.NoError(t, err)

	// On POSIX, the mode is preserved exactly.
	assert.Equal(t, os.FileMode(0o644), info.Mode()&os.ModePerm,
		"file mode must be preserved on POSIX")
}

// TestSyncLock_NoCrossProcessExclusionOnWindows documents that the sync
// lock is a no-op on Windows (synclock_windows.go), so no test may assume
// cross-process mutual exclusion there. On POSIX, a second acquire fails;
// on Windows it succeeds silently. This test asserts the POSIX behaviour
// and skips on Windows with a visible reason, mirroring the existing
// TestSyncLock_DoubleAcquireFailsOnUnix precedent.
//
// Fulfills VAL-REGRESSION-013(c) for the sync lock.
func TestSyncLock_NoCrossProcessExclusionOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sync lock is a no-op on Windows (synclock_windows.go); cross-process mutual exclusion is not available — tracked in RAPTOR-16928")
	}

	dir := setupProject(t)

	lock1, err := AcquireSyncLock(dir)
	require.NoError(t, err)

	t.Cleanup(func() { _ = lock1.Release() })

	// A second acquire must fail on POSIX — the lock provides mutual
	// exclusion. On Windows this would succeed (no-op lock), so the
	// test skips rather than asserting a vacuous pass.
	_, err = AcquireSyncLock(dir)
	assert.Error(t, err,
		"second acquire must fail on POSIX while the first is held")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// itoa converts an int to its decimal string representation without pulling
// in strconv (kept lightweight for a test helper used in subtest names).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [20]byte

	i := len(buf)

	for n > 0 {
		i--

		buf[i] = byte('0' + n%10)
		n /= 10
	}

	return string(buf[i:])
}
