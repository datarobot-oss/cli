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
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock in the fact that change detection is purely content-hash
// based. The ticket (RAPTOR-19525) alleged a size+mtime fast path that silently
// skips files whose content changed but whose size and mtime are preserved.
// That mechanism does not exist in this codebase: Diff receives only hashes,
// and hashEntries rehashes every file every run. These tests would fail loudly
// if anyone ever introduced such a shortcut as a "performance improvement".

// regressionEngine builds an engine over a syncedProject directory wired to a
// draft artifact whose codeRef matches the manifest's catalogID and versionID.
// The returned fakeFilesClient lets tests inspect call counters; the directory
// lets tests modify files on disk between Plan calls.
func regressionEngine(t *testing.T, files map[string]string) (*Engine, *fakeFilesClient, string) {
	t.Helper()

	const (
		catalogID = "cid-regression"
		versionID = "ver-regression"
	)

	dir := syncedProject(t, files, catalogID, versionID)

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-regression",
		versionID: "ver-regression-next",
	}

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e, fake, dir
}

// uploadPathSet returns the set of paths in the upload portion of the plan.
func uploadPathSet(plan *SyncPlan) map[string]struct{} {
	out := make(map[string]struct{}, len(plan.Uploads))
	for _, fa := range plan.Uploads {
		out[fa.Path] = struct{}{}
	}

	return out
}

// TestSameSizeSameMtimeDetected is the codified refutation of RAPTOR-19525's
// stated mechanism. A file is rewritten to different bytes of identical length,
// and its mtime is restored to the pre-change value via os.Chtimes. If change
// detection were size+mtime based (as the ticket alleged), this file would be
// invisible. Because detection is content-hash based, it appears in the plan
// as a modification to upload.
//
// Fulfills VAL-REGRESSION-001.
func TestSameSizeSameMtimeDetected(t *testing.T) {
	const (
		original = "print('A')\n" // 12 bytes
		modified = "print('B')\n" // 12 bytes, different content
	)

	e, _, dir := regressionEngine(t, map[string]string{
		"app.py": original,
	})

	abs := filepath.Join(dir, "app.py")

	// Capture the original mtime before modification.
	info, err := os.Stat(abs)
	require.NoError(t, err)

	origMtime := info.ModTime()

	// Rewrite to different content of the same byte length.
	require.NoError(t, os.WriteFile(abs, []byte(modified), 0o644))

	// Restore the original mtime so both size and mtime match the pre-change
	// state. This is the exact scenario the ticket alleged would be skipped.
	require.NoError(t, os.Chtimes(abs, origMtime, origMtime))

	// Confirm the mtime was actually restored (some filesystems have coarse
	// mtime resolution, so verify rather than assume).
	postInfo, err := os.Stat(abs)
	require.NoError(t, err)

	assert.True(t, postInfo.ModTime().Equal(origMtime),
		"mtime must be restored for the test to be meaningful")

	plan, err := e.Plan()
	require.NoError(t, err)

	paths := uploadPathSet(plan)
	_, present := paths["app.py"]
	assert.True(t, present,
		"same-size same-mtime content change must be detected as a modification; "+
			"if this fails, a size+mtime fast path was introduced")
}

// TestDetectionIsContentOnly is a 2x2 matrix over {size matches manifest, size
// differs} x {content matches manifest, content differs}. Detection must fire
// if and only if content differs, in every cell. This proves detection is
// purely content-hash based and never consults size or mtime.
//
// The "size differs, content matches" cell is synthetic: the manifest's size
// field is manually corrupted while the hash stays correct. A size-based fast
// path would flag this file as changed; the hash-based mechanism correctly
// leaves it alone.
//
// Fulfills VAL-REGRESSION-005.
func TestDetectionIsContentOnly(t *testing.T) {
	const (
		original = "print('A')\n"  // 12 bytes
		modified = "print('B')\n"  // 12 bytes, same size, different content
		grown    = "print('BB')\n" // 13 bytes, different size and content
	)

	cases := []struct {
		name         string
		setup        func(t *testing.T, dir string)
		wantDetected bool
	}{
		{
			name: "size_matches_content_matches",
			// File unchanged: size and content both match the manifest.
			// Detection must NOT fire.
			setup:        func(_ *testing.T, _ string) {},
			wantDetected: false,
		},
		{
			name: "size_matches_content_differs",
			// Same-size content change: the ticket's exact scenario.
			// Detection MUST fire.
			setup: func(t *testing.T, dir string) {
				t.Helper()

				modifyFile(t, dir, "app.py", modified)
			},
			wantDetected: true,
		},
		{
			name: "size_differs_content_matches",
			// Manifest's size field is corrupted while the hash stays correct.
			// A size-based check would flag this; hash-based detection must NOT.
			setup: func(t *testing.T, dir string) {
				t.Helper()

				corruptManifestSize(t, dir, "app.py", 9999)
			},
			wantDetected: false,
		},
		{
			name: "size_differs_content_differs",
			// Normal modification: both size and content change.
			// Detection MUST fire.
			setup: func(t *testing.T, dir string) {
				t.Helper()

				modifyFile(t, dir, "app.py", grown)
			},
			wantDetected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _, dir := regressionEngine(t, map[string]string{
				"app.py": original,
			})

			tc.setup(t, dir)

			plan, err := e.Plan()
			require.NoError(t, err)

			paths := uploadPathSet(plan)
			_, present := paths["app.py"]
			assert.Equal(t, tc.wantDetected, present,
				"detection must fire iff content differs, regardless of size")
		})
	}
}

// TestDetectionControls provides true-positive and false-positive controls so
// the detection assertions above cannot be satisfied trivially.
//
// True positives:
//   - size_and_content_change: both size and content change → detected.
//   - large_file_content_change: a 4 MiB file whose content changes → detected,
//     guarding against a future size-threshold shortcut for large files.
//
// False positives:
//   - untouched_file: a file identical to its manifest entry → absent from plan.
//   - mtime_only_advance: content unchanged, mtime advanced → not detected.
//
// Fulfills VAL-REGRESSION-003 and VAL-REGRESSION-004.
func TestDetectionControls(t *testing.T) {
	t.Run("size_and_content_change", func(t *testing.T) {
		e, _, dir := regressionEngine(t, map[string]string{
			"app.py": "print('A')\n",
		})

		modifyFile(t, dir, "app.py", "print('hello world')\n")

		plan, err := e.Plan()
		require.NoError(t, err)

		paths := uploadPathSet(plan)
		_, present := paths["app.py"]
		assert.True(t, present, "a size-and-content change must be detected")
	})

	t.Run("large_file_content_change", func(t *testing.T) {
		// A 4 MiB file guards against a future size-threshold shortcut that
		// skips hashing for "large" files under the assumption they are
		// unlikely to change without a size change.
		const largeSize = 4 * 1024 * 1024 // 4 MiB

		original := strings.Repeat("A", largeSize)
		modified := strings.Repeat("B", largeSize)

		e, _, dir := regressionEngine(t, map[string]string{
			"big.bin": original,
		})

		modifyFile(t, dir, "big.bin", modified)

		plan, err := e.Plan()
		require.NoError(t, err)

		paths := uploadPathSet(plan)
		_, present := paths["big.bin"]
		assert.True(t, present, "a multi-megabyte content change must be detected")
	})

	t.Run("untouched_file", func(t *testing.T) {
		e, _, _ := regressionEngine(t, map[string]string{
			"app.py": "print('A')\n",
		})

		// No modifications: the file is identical to its manifest entry.
		plan, err := e.Plan()
		require.NoError(t, err)

		paths := uploadPathSet(plan)
		_, present := paths["app.py"]
		assert.False(t, present,
			"an untouched file must not appear in the upload plan")
	})

	t.Run("mtime_only_advance", func(t *testing.T) {
		e, _, dir := regressionEngine(t, map[string]string{
			"app.py": "print('A')\n",
		})

		abs := filepath.Join(dir, "app.py")

		// Advance the mtime without changing the content.
		future := time.Now().Add(2 * time.Hour)
		require.NoError(t, os.Chtimes(abs, future, future))

		plan, err := e.Plan()
		require.NoError(t, err)

		paths := uploadPathSet(plan)
		_, present := paths["app.py"]
		assert.False(t, present,
			"an mtime-only advance with identical content must not be detected")
	})
}

// TestPlanRowSizesMatchDisk asserts that each upload plan row's displayed byte
// size (fa.LocalSize) equals the real on-disk file size at plan time. This
// ensures the sizes a validator reads off the plan can be trusted as evidence.
//
// Fulfills VAL-REGRESSION-014.
func TestPlanRowSizesMatchDisk(t *testing.T) {
	files := map[string]string{
		"app.py":      "print('A')\n",
		"config.yaml": "name: test\n",
		"data.bin":    "ABCDEF",
	}

	e, _, dir := regressionEngine(t, files)

	// Modify files to different sizes so the plan is non-empty and the
	// sizes on disk differ from the manifest's recorded sizes.
	modifyFile(t, dir, "app.py", "print('hello world')\n")
	modifyFile(t, dir, "data.bin", "ABCDEFGHIJKL")

	plan, err := e.Plan()
	require.NoError(t, err)

	require.NotEmpty(t, plan.Uploads, "plan must have uploads for the size assertion to be meaningful")

	for _, fa := range plan.Uploads {
		abs := filepath.Join(dir, filepath.FromSlash(fa.Path))
		info, err := os.Stat(abs)
		require.NoError(t, err, "stat %s", fa.Path)

		assert.Equal(t, info.Size(), fa.LocalSize,
			"upload plan row for %s: displayed size must equal real on-disk size", fa.Path)
	}
}

// corruptManifestSize loads the manifest, changes the size field for the given
// path to a wrong value while keeping the hash correct, and re-saves it. This
// creates a synthetic "size differs, content matches" state for the detection
// matrix: the file on disk is unchanged (same hash), but the manifest's size
// field no longer matches the real size. A size-based fast path would flag
// this file as changed; the hash-based mechanism correctly leaves it alone.
func corruptManifestSize(t *testing.T, dir, rel string, wrongSize int64) {
	t.Helper()

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	fm, ok := manifest.Files[rel]
	require.True(t, ok, "manifest must contain %s", rel)

	// Keep the hash, corrupt only the size.
	fm.Size = wrongSize
	manifest.Files[rel] = fm

	require.NoError(t, wapi.SaveManifest(dir, manifest))
}
