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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase2_SymlinkNotice_EmittedFromPhase2 verifies that the skipped-symlink
// warning is emitted via log.Warn from within Phase 2, so it survives a later
// phase failing. The warning must name the symlink and say it was NOT
// uploaded or synced.
//
// Fulfills VAL-SYMLINK-001(d,e) and VAL-SYMLINK-011(f) at the go test level.
func TestPhase2_SymlinkNotice_EmittedFromPhase2(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// A file symlink and a directory symlink.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realfile.py"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realfile.py"),
		filepath.Join(dir, "link_to_file.py")))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "realdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "realdir", "inner.py"), []byte("y"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realdir"),
		filepath.Join(dir, "link_to_dir")))

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	// Both symlinks are named in the warning.
	assert.Contains(t, logged, "link_to_file.py")
	assert.Contains(t, logged, "link_to_dir")

	// The file-symlink wording says "not uploaded".
	assert.Contains(t, logged, "was not uploaded",
		"the file-symlink notice must say the symlink was not uploaded")

	// The directory-symlink wording says "not synced" and conveys that a
	// whole subtree is omitted.
	assert.Contains(t, logged, "was not synced",
		"the directory-symlink notice must say the symlink was not synced")
	assert.Contains(t, logged, "subtree",
		"the directory-symlink wording must convey that a whole subtree is omitted")

	// The file and directory wordings differ.
	assert.NotEqual(t,
		strings.Index(logged, "was not uploaded"),
		strings.Index(logged, "was not synced"),
		"file-symlink wording must differ from directory-symlink wording")
}

// TestPhase2_SymlinkNotice_SurvivesLaterPhaseFailure verifies that the
// skipped-symlink warning reaches stderr even when a later phase fails, so
// the user hears it exactly when they need it most. The warning is emitted
// from within Phase 2 via log.Warn, not after a successful plan.
//
// Fulfills VAL-SYMLINK-011(f).
func TestPhase2_SymlinkNotice_SurvivesLaterPhaseFailure(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	require.NoError(t, os.WriteFile(filepath.Join(dir, "realfile.py"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "realfile.py"),
		filepath.Join(dir, "link_to_file.py")))

	// Make a file unreadable so hashEntries fails after the walk and the
	// log.Warn emission. The walk (filepath.WalkDir) only needs directory
	// execute permission to enumerate entries; it does not open individual
	// files. hashEntries then opens each file to hash it, and an unreadable
	// file produces a "permission denied" error that fails Phase 2 — but
	// the symlink warning has already been emitted to stderr by then.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unreadable.py"), []byte("x"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(dir, "unreadable.py"), 0o000))

	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "unreadable.py"), 0o644) })

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.Error(t, err, "hashEntries must fail on an unreadable file")
	})

	// The symlink warning still appears despite the phase failure.
	assert.Contains(t, logged, "link_to_file.py",
		"the symlink warning must survive a later phase failing")
	assert.Contains(t, logged, "was not uploaded",
		"the warning must say the symlink was not uploaded")
}

// TestPhase2_SymlinkNotice_BoundedAtFive verifies that when more than
// SymlinkNoticeBound symlinks are skipped, the stderr prose lists the first
// SymlinkNoticeBound in deterministic order followed by a count of the
// remainder, while the engine's skippedSymlinks field carries every one.
//
// Fulfills VAL-SYMLINK-009(a).
func TestPhase2_SymlinkNotice_BoundedAtFive(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// Create a real file to point the symlinks at.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.py"), []byte("x"), 0o644))

	// Create 7 file symlinks (exceeding the bound of 5).
	for i := 0; i < 7; i++ {
		name := "link" + string(rune('a'+i)) + ".py"
		require.NoError(t, os.Symlink(
			filepath.Join(dir, "real.py"),
			filepath.Join(dir, name)))
	}

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	// The first 5 symlinks appear in the log.
	for i := 0; i < 5; i++ {
		name := "link" + string(rune('a'+i)) + ".py"
		assert.Contains(t, logged, name,
			"the first 5 symlinks must appear in the bounded prose")
	}

	// The 6th and 7th do not appear individually.
	assert.NotContains(t, logged, "linkf.py",
		"the 6th symlink must not appear in the bounded prose")
	assert.NotContains(t, logged, "linkg.py",
		"the 7th symlink must not appear in the bounded prose")

	// The count of the remainder appears.
	assert.Contains(t, logged, "2 more",
		"the count of the remainder must appear in the prose")

	// The engine's skippedSymlinks field carries every symlink.
	require.Len(t, e.skippedSymlinks, 7,
		"the structured field must list every symlink even when prose is bounded")
}

// TestPhase2_SymlinkNotice_NoSymlinksNoNotice verifies that a project with no
// symlinks emits no symlink-related warning.
//
// Fulfills VAL-SYMLINK-005(a) at the go test level.
func TestPhase2_SymlinkNotice_NoSymlinksNoNotice(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	assert.NotContains(t, logged, "symlink",
		"a project with no symlinks must emit no symlink-related warning")
	assert.Empty(t, e.skippedSymlinks,
		"the skippedSymlinks field must be empty when there are no symlinks")
}

// TestPhase2_SymlinkNotice_DeterministicOrder verifies that the skipped
// symlinks are sorted by path on the engine, so notices and the structured
// field are deterministic across runs.
func TestPhase2_SymlinkNotice_DeterministicOrder(t *testing.T) {
	skipNonWindowsSymlink(t)

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.py"), []byte("x"), 0o644))

	// Create symlinks in non-sorted order of creation.
	for _, name := range []string{"z_link.py", "a_link.py", "m_link.py"} {
		require.NoError(t, os.Symlink(
			filepath.Join(dir, "real.py"),
			filepath.Join(dir, name)))
	}

	e := lockfileEngine(t, dir, noLockfileRunner)

	_, err := e.Plan()
	require.NoError(t, err)

	// The skippedSymlinks on the engine are sorted by path.
	require.Len(t, e.skippedSymlinks, 3)
	assert.Equal(t, "a_link.py", e.skippedSymlinks[0].Path)
	assert.Equal(t, "m_link.py", e.skippedSymlinks[1].Path)
	assert.Equal(t, "z_link.py", e.skippedSymlinks[2].Path)
}
