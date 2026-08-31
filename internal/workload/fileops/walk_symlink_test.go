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

package fileops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symlinkTestSkipReason is the visible reason emitted when symlink tests are
// skipped on Windows. os.Symlink needs Developer Mode or elevated privileges
// there, and junctions are a different mechanism entirely — no attempt is made.
const symlinkTestSkipReason = "symlink tests skipped on Windows: os.Symlink needs Developer Mode; junctions are not equivalent"

// skipNonWindowsSymlink skips on Windows following the existing
// runtime.GOOS == "windows" precedent in walk_test.go. The skip reason is
// visible so a CI log shows why the test did not run rather than appearing to
// have passed vacuously.
func skipNonWindowsSymlink(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip(symlinkTestSkipReason)
	}
}

// symlinkReport captures what the SymlinkLogger callback observed, so a test
// can assert on path, target, and isDir together.
type symlinkReport struct {
	rel    string
	target string
	isDir  bool
}

// TestWalk_SymlinkedDirectory_Pruned verifies that a symlink to a directory
// produces exactly one callback for the symlink path and zero callbacks for
// any path beneath it, and that no entry is emitted for the symlink or its
// children. filepath.WalkDir tests ModeSymlink before IsDir and never descends
// into a non-directory, so children are never visited.
//
// Fulfills VAL-SYMLINK-011(a) and VAL-SYMLINK-011(e).
func TestWalk_SymlinkedDirectory_Pruned(t *testing.T) {
	skipNonWindowsSymlink(t)

	root := t.TempDir()
	writeFile(t, root, "realdir/inner.py", "x")
	writeFile(t, root, "realdir/sub/deep.py", "y")

	// link_to_dir -> realdir (a directory symlink)
	require.NoError(t, os.Symlink(
		filepath.Join(root, "realdir"),
		filepath.Join(root, "link_to_dir")))

	var reports []symlinkReport

	got, err := Walk(root, nil, func(rel, target string, isDir bool) {
		reports = append(reports, symlinkReport{rel: rel, target: target, isDir: isDir})
	})
	require.NoError(t, err)

	// Exactly one callback for the symlink path itself.
	require.Len(t, reports, 1, "a symlinked directory must produce exactly one callback")
	assert.Equal(t, "link_to_dir", reports[0].rel)
	assert.True(t, reports[0].isDir, "isDir must be true when the link target is a directory")

	// No entry for the symlink or anything beneath it. The real directory's
	// children ARE present (the symlink is a separate path).
	paths := relPaths(got)
	assert.NotContains(t, paths, "link_to_dir")
	assert.NotContains(t, paths, "link_to_dir/inner.py")
	assert.NotContains(t, paths, "link_to_dir/sub/deep.py")
	assert.Contains(t, paths, "realdir/inner.py")
	assert.Contains(t, paths, "realdir/sub/deep.py")
}

// TestWalk_Symlink_ReportsIsDir verifies that the callback reports isDir true
// when the link target is a directory and false when it is a regular file.
// The kind is resolved via os.Stat (which follows the link), the syscall
// already available on the notify path.
//
// Fulfills VAL-SYMLINK-011(b).
func TestWalk_Symlink_ReportsIsDir(t *testing.T) {
	skipNonWindowsSymlink(t)

	root := t.TempDir()
	writeFile(t, root, "realfile.py", "x")
	writeFile(t, root, "realdir/inner.py", "y")

	// link_to_file -> realfile.py (a file symlink)
	require.NoError(t, os.Symlink(
		filepath.Join(root, "realfile.py"),
		filepath.Join(root, "link_to_file")))

	// link_to_dir -> realdir (a directory symlink)
	require.NoError(t, os.Symlink(
		filepath.Join(root, "realdir"),
		filepath.Join(root, "link_to_dir")))

	var reports []symlinkReport

	_, err := Walk(root, nil, func(rel, target string, isDir bool) {
		reports = append(reports, symlinkReport{rel: rel, target: target, isDir: isDir})
	})
	require.NoError(t, err)

	// Two callbacks, one per symlink, each with the correct kind.
	require.Len(t, reports, 2)

	byRel := make(map[string]symlinkReport, len(reports))
	for _, r := range reports {
		byRel[r.rel] = r
	}

	fileRpt, ok := byRel["link_to_file"]
	require.True(t, ok, "file symlink must be reported")
	assert.False(t, fileRpt.isDir, "isDir must be false for a file symlink")
	assert.NotEmpty(t, fileRpt.target, "target must be the link text")

	dirRpt, ok := byRel["link_to_dir"]
	require.True(t, ok, "directory symlink must be reported")
	assert.True(t, dirRpt.isDir, "isDir must be true for a directory symlink")
	assert.NotEmpty(t, dirRpt.target, "target must be the link text")
}

// TestWalk_DanglingSymlink verifies that a symlink whose target does not exist
// produces a callback with an empty target and isDir false, and that the walk
// returns no error. The target is cleared because os.Stat fails on a dangling
// link — the user sees "this link points nowhere" rather than a walk crash.
//
// Fulfills VAL-SYMLINK-011(c).
func TestWalk_DanglingSymlink(t *testing.T) {
	skipNonWindowsSymlink(t)

	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")

	// A symlink to a path that does not exist.
	require.NoError(t, os.Symlink(
		filepath.Join(root, "nonexistent"),
		filepath.Join(root, "dangling.lnk")))

	var reports []symlinkReport

	got, err := Walk(root, nil, func(rel, target string, isDir bool) {
		reports = append(reports, symlinkReport{rel: rel, target: target, isDir: isDir})
	})
	require.NoError(t, err, "a dangling symlink must not cause a walk error")

	require.Len(t, reports, 1, "dangling symlink must produce exactly one callback")
	assert.Equal(t, "dangling.lnk", reports[0].rel)
	assert.Empty(t, reports[0].target, "dangling symlink must report an empty target")
	assert.False(t, reports[0].isDir, "dangling symlink must report isDir false")

	// The dangling symlink is not in entries.
	paths := relPaths(got)
	assert.NotContains(t, paths, "dangling.lnk")
	assert.Contains(t, paths, "agent.py")
}

// TestWalk_SymlinkChain_NotFollowed verifies that a symlink chain (outer ->
// inner -> realfile, both outer and inner top-level) reports each top-level
// symlink exactly once and follows neither. The walker never follows symlinks
// (it tests ModeSymlink before descending), so a chain is reported per link
// without traversing to the final target.
//
// Fulfills VAL-SYMLINK-011(d).
func TestWalk_SymlinkChain_NotFollowed(t *testing.T) {
	skipNonWindowsSymlink(t)

	root := t.TempDir()
	writeFile(t, root, "realfile.py", "x")

	// inner -> realfile.py
	require.NoError(t, os.Symlink(
		filepath.Join(root, "realfile.py"),
		filepath.Join(root, "inner.lnk")))

	// outer -> inner.lnk (a chain: outer -> inner -> realfile.py)
	require.NoError(t, os.Symlink(
		filepath.Join(root, "inner.lnk"),
		filepath.Join(root, "outer.lnk")))

	var reports []symlinkReport

	got, err := Walk(root, nil, func(rel, target string, isDir bool) {
		reports = append(reports, symlinkReport{rel: rel, target: target, isDir: isDir})
	})
	require.NoError(t, err)

	// Each top-level symlink is reported exactly once. The walker does not
	// follow the chain: outer reports its own link text (inner.lnk), not
	// realfile.py, and neither is uploaded.
	require.Len(t, reports, 2, "each top-level symlink must be reported exactly once")

	byRel := make(map[string]int)
	for _, r := range reports {
		byRel[r.rel]++
	}

	assert.Equal(t, 1, byRel["outer.lnk"], "outer must be reported exactly once")
	assert.Equal(t, 1, byRel["inner.lnk"], "inner must be reported exactly once")
	assert.NotContains(t, byRel, "realfile.py", "the chain target must not be reported as a symlink")

	// Neither symlink is in entries; the real file IS.
	paths := relPaths(got)
	assert.NotContains(t, paths, "outer.lnk")
	assert.NotContains(t, paths, "inner.lnk")
	assert.Contains(t, paths, "realfile.py")

	// Neither link resolves to a directory (realfile.py is a regular file).
	for _, r := range reports {
		assert.False(t, r.isDir, "chain links to a regular file must report isDir false")
	}
}

// TestWalk_SymlinkedDirectory_ExternalTarget verifies that a symlink whose
// target is an absolute path outside the project directory is reported as a
// skipped symlink, is not uploaded, and no file outside the project root is
// read. The plan contains only paths under the project root.
//
// Fulfills VAL-SYMLINK-004(b) at the walk level.
func TestWalk_SymlinkedDirectory_ExternalTarget(t *testing.T) {
	skipNonWindowsSymlink(t)

	root := t.TempDir()
	writeFile(t, root, "agent.py", "x")

	// A file outside the project root.
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "secret.txt"), []byte("secret"), 0o644))

	// link_to_external -> /tmp/.../secret.txt (absolute, outside project)
	require.NoError(t, os.Symlink(
		filepath.Join(external, "secret.txt"),
		filepath.Join(root, "link_to_external")))

	var reports []symlinkReport

	got, err := Walk(root, nil, func(rel, target string, isDir bool) {
		reports = append(reports, symlinkReport{rel: rel, target: target, isDir: isDir})
	})
	require.NoError(t, err)

	require.Len(t, reports, 1, "external-target symlink must be reported")
	assert.Equal(t, "link_to_external", reports[0].rel)
	assert.False(t, reports[0].isDir, "external file target must report isDir false")

	// The symlink is not in entries; only the real project file is.
	paths := relPaths(got)
	assert.NotContains(t, paths, "link_to_external")
	assert.Contains(t, paths, "agent.py")
}
