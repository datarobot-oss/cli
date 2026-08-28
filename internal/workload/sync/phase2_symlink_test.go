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
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipNonWindowsSymlink skips on Windows following the existing
// runtime.GOOS == "windows" precedent. The skip reason is visible so CI shows
// why the test did not run.
func skipNonWindowsSymlink(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symlink tests skipped on Windows: os.Symlink needs Developer Mode; junctions are not equivalent")
	}
}

// symlinkPaths extracts the Path field from each SkippedSymlink on the engine.
func symlinkPaths(ss []SkippedSymlink) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Path
	}

	return out
}

// symlinkIsDir extracts the IsDir field for a given path from the engine's
// skippedSymlinks, returning false if the path is absent.
func symlinkIsDir(ss []SkippedSymlink, path string) bool {
	for _, s := range ss {
		if s.Path == path {
			return s.IsDir
		}
	}

	return false
}

// TestPhase2_Symlink_FilteredByDrignore verifies that a symlink matching a
// .drignore pattern is neither uploaded nor reported in the skipped-symlink
// list. The walk's symlink arm returns before the ignore check, so filtering
// must happen at the collection site in phase2_manifests.go via the same
// matcher used for regular files.
//
// Fulfills VAL-SYMLINK-002 (go test portion).
func TestPhase2_Symlink_FilteredByDrignore(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// realfile.py is a real file; link_to_file.py is a symlink to it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realfile.py"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realfile.py"),
		filepath.Join(dir, "link_to_file.py")))

	// realdir is a real directory with a child; link_to_dir is a symlink to it.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "realdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realdir", "inner.py"), []byte("y"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realdir"),
		filepath.Join(dir, "link_to_dir")))

	// .drignore excludes both the file symlink and the directory symlink
	// (in both bare and trailing-slash form, since matcher.Match handles
	// the slash for isDir=true).
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName),
		[]byte("link_to_file.py\nlink_to_dir/\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Neither symlink is in the upload plan.
	uploadPaths := uploadPathsOf(plan)
	assert.NotContains(t, uploadPaths, "link_to_file.py")
	assert.NotContains(t, uploadPaths, "link_to_dir")
	assert.NotContains(t, uploadPaths, "link_to_dir/inner.py")

	// Neither ignored symlink is in the skipped-symlink list.
	skipped := symlinkPaths(e.skippedSymlinks)
	assert.NotContains(t, skipped, "link_to_file.py",
		"a .drignore-matched file symlink must not be reported")
	assert.NotContains(t, skipped, "link_to_dir",
		"a .drignore-matched directory symlink must not be reported")

	// The real files are still in the plan.
	assert.Contains(t, uploadPaths, "app.py")
	assert.Contains(t, uploadPaths, "realfile.py")
	assert.Contains(t, uploadPaths, "realdir/inner.py")
}

// TestPhase2_Symlink_SystemExcludedNotReported verifies that a symlink whose
// relative path is a system-excluded path (e.g. named .git, or living under
// .datarobot/workload) is not reported in the skipped-symlink list. System
// excludes are hardcoded in the ignore matcher and fold case, so a symlink
// named .git is excluded the same way a real .git directory would be.
//
// Fulfills VAL-SYMLINK-003.
func TestPhase2_Symlink_SystemExcludedNotReported(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// A real directory to point the symlinks at.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "realdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realdir", "inner.py"), []byte("y"), 0o644))

	// .git symlink -> realdir (system-excluded name)
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realdir"),
		filepath.Join(dir, ".git")))

	// .datarobot/workload/ is created by wapi.Initialize as a real directory.
	// A symlink placed inside it is system-excluded by prefix (.datarobot/workload/...).
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realdir"),
		filepath.Join(dir, ".datarobot", "workload", "link_to_dir")))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Neither system-excluded symlink is in the upload plan.
	uploadPaths := uploadPathsOf(plan)
	assert.NotContains(t, uploadPaths, ".git")
	assert.NotContains(t, uploadPaths, ".datarobot/workload/link_to_dir")

	// Neither system-excluded symlink is in the skipped-symlink list.
	skipped := symlinkPaths(e.skippedSymlinks)
	assert.NotContains(t, skipped, ".git",
		"a symlink named .git must not be reported (system-excluded)")
	assert.NotContains(t, skipped, ".datarobot/workload/link_to_dir",
		"a symlink under .datarobot/workload must not be reported (system-excluded)")

	// The real files are still in the plan.
	assert.Contains(t, uploadPaths, "app.py")
	assert.Contains(t, uploadPaths, "realdir/inner.py")
}

// TestPhase2_Symlink_InsideDrignorePrunedDirectory verifies that a symlink
// inside a .drignore-pruned real directory is not reported and none of its
// contents are uploaded. The walk prunes the directory before descending, so
// the symlink inside it is never visited and never produces a callback.
//
// Fulfills VAL-SYMLINK-002(c).
func TestPhase2_Symlink_InsideDrignorePrunedDirectory(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// A real directory with a symlink inside it.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ignored", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored", "real.py"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "ignored", "real.py"),
		filepath.Join(dir, "ignored", "link.py")))

	// .drignore prunes the entire ignored/ directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName),
		[]byte("ignored/\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Nothing under ignored/ is in the upload plan.
	uploadPaths := uploadPathsOf(plan)
	assert.NotContains(t, uploadPaths, "ignored/real.py")
	assert.NotContains(t, uploadPaths, "ignored/link.py")

	// The symlink inside the pruned directory is not in the skipped-symlink
	// list (the walk never visited it).
	skipped := symlinkPaths(e.skippedSymlinks)
	assert.NotContains(t, skipped, "ignored/link.py",
		"a symlink inside a .drignore-pruned directory must not be reported")

	// The real file outside the pruned directory is still in the plan.
	assert.Contains(t, uploadPaths, "app.py")
}

// TestPhase2_Symlink_ReportedWithIsDir verifies that the engine's
// skippedSymlinks carries the correct isDir for each reported symlink: true
// for a directory symlink and false for a file symlink. This is the
// phase2-level complement to the walk-level isDir tests.
//
// Fulfills VAL-SYMLINK-011(b) at the collection site.
func TestPhase2_Symlink_ReportedWithIsDir(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// realfile.py and a file symlink to it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realfile.py"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realfile.py"),
		filepath.Join(dir, "link_to_file.py")))

	// realdir with a child and a directory symlink to it.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "realdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realdir", "inner.py"), []byte("y"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realdir"),
		filepath.Join(dir, "link_to_dir")))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Neither symlink is in the upload plan.
	uploadPaths := uploadPathsOf(plan)
	assert.NotContains(t, uploadPaths, "link_to_file.py")
	assert.NotContains(t, uploadPaths, "link_to_dir")

	// Both symlinks are reported with the correct isDir.
	skipped := e.skippedSymlinks
	assert.Contains(t, symlinkPaths(skipped), "link_to_file.py")
	assert.Contains(t, symlinkPaths(skipped), "link_to_dir")
	assert.False(t, symlinkIsDir(skipped, "link_to_file.py"),
		"file symlink must report isDir false")
	assert.True(t, symlinkIsDir(skipped, "link_to_dir"),
		"directory symlink must report isDir true")

	// The real files are still in the plan.
	assert.Contains(t, uploadPaths, "app.py")
	assert.Contains(t, uploadPaths, "realfile.py")
	assert.Contains(t, uploadPaths, "realdir/inner.py")
}

// TestPhase2_Symlink_DanglingReported verifies that a dangling symlink is
// reported in the skipped-symlink list with isDir false, and that the sync
// completes without error. The target is empty because os.Stat fails on a
// dangling link.
//
// Fulfills VAL-SYMLINK-004(a) at the go test level.
func TestPhase2_Symlink_DanglingReported(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// A symlink to a path that does not exist.
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "nonexistent"),
		filepath.Join(dir, "dangling.lnk")))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err, "a dangling symlink must not fail the sync")

	// The dangling symlink is not in the upload plan.
	assert.NotContains(t, uploadPathsOf(plan), "dangling.lnk")

	// The dangling symlink IS reported in the skipped-symlink list with
	// isDir false.
	skipped := e.skippedSymlinks
	assert.Contains(t, symlinkPaths(skipped), "dangling.lnk")
	assert.False(t, symlinkIsDir(skipped, "dangling.lnk"),
		"dangling symlink must report isDir false")
}
