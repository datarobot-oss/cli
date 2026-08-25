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

//go:build !windows

package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriteFile_CreatesNew(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "new.json")

	err := AtomicWriteFile(target, []byte(`{"hello":"world"}`))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.JSONEq(t, `{"hello":"world"}`, string(got))

	assertSameModeAsPlainWrite(t, target)
}

// assertSameModeAsPlainWrite compares against a file the standard library
// just created with the same requested mode, so the assertion holds under any
// umask rather than only the developer's. It is also the contract: a file the
// CLI writes should be no wider than one the user's own tools would write.
func assertSameModeAsPlainWrite(t *testing.T, path string) {
	t.Helper()

	reference := filepath.Join(t.TempDir(), "reference")
	require.NoError(t, os.WriteFile(reference, nil, DefaultFileMode))

	want, err := os.Stat(reference)
	require.NoError(t, err)

	got, err := os.Stat(path)
	require.NoError(t, err)

	assert.Equal(t, want.Mode().Perm(), got.Mode().Perm())
}

// targetMode used to measure the umask by creating the destination itself,
// which meant any failure after that point left an empty file where there had
// been none: exactly the state AtomicWriteFile promises never to produce.
// Nothing it does may touch the destination, and the scratch path it measures
// on has to be gone by the time it returns.
func TestTargetMode_LeavesTheDestinationAlone(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.json")
	probe := filepath.Join(dir, "new.json.probe")

	mode, err := targetMode(target, probe)
	require.NoError(t, err)
	assert.NotZero(t, mode)

	assert.NoFileExists(t, target)
	assert.NoFileExists(t, probe)
}

// When the umask cannot be measured the answer has to narrow rather than
// widen. Returning DefaultFileMode here would chmod the file to 0644 whatever
// the umask says, which is the widening the measurement exists to prevent.
func TestTargetMode_NarrowsWhenTheProbeFails(t *testing.T) {
	dir := t.TempDir()
	taken := filepath.Join(dir, "taken")
	require.NoError(t, os.WriteFile(taken, nil, DefaultFileMode))

	mode, err := targetMode(filepath.Join(dir, "new.json"), taken)
	require.NoError(t, err)
	assert.Equal(t, ownerOnlyFileMode, mode)
}

func TestAtomicWriteFile_LeavesNoScratchFiles(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, AtomicWriteFile(filepath.Join(dir, "new.json"), []byte("payload")))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the write left scratch files behind: %v", entries)
}

// A file that already exists keeps the mode it had: tightening a manifest to
// 0600 should survive an unrelated edit such as the workload-id write-back.
func TestAtomicWriteFile_PreservesAnExistingMode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "tightened")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	require.NoError(t, AtomicWriteFile(target, []byte("new")))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "existing.json")

	err := os.WriteFile(target, []byte("old"), 0o644)
	require.NoError(t, err)

	err = AtomicWriteFile(target, []byte("new"))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// When path is a symlink, the write must land in the real file and leave the
// link intact. Symlink creation on Windows requires elevated privileges, so
// this test lives in the !windows file.
func TestAtomicWriteFile_WriteThroughSymlink(t *testing.T) {
	realDir := t.TempDir()
	linkDir := t.TempDir()

	realFile := filepath.Join(realDir, "manifest.yaml")
	linkPath := filepath.Join(linkDir, ".datarobot.yaml")

	// Create the real file and a symlink pointing at it.
	require.NoError(t, os.WriteFile(realFile, []byte("original"), 0o644))
	require.NoError(t, os.Symlink(realFile, linkPath))

	// Write through the symlink.
	require.NoError(t, AtomicWriteFile(linkPath, []byte("updated")))

	// The symlink must still exist (not be replaced by a regular file).
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0), fi.Mode()&os.ModeSymlink, "link path should still be a symlink, got mode %v", fi.Mode())

	// The real file must contain the new content.
	got, err := os.ReadFile(realFile)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(got))

	// The link must still resolve to the real file.
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, realFile, target)
}

// A symlink whose target is spelled relative to the link's own directory (the
// shape dotfiles tools like GNU stow produce) must resolve against that
// directory, and the link's relative spelling must survive the write.
func TestAtomicWriteFile_WriteThroughRelativeSymlink(t *testing.T) {
	dir := t.TempDir()

	realFile := filepath.Join(dir, "manifest.yaml")
	linkPath := filepath.Join(dir, ".datarobot.yaml")

	// The link target is the bare filename: relative to the link's directory.
	require.NoError(t, os.WriteFile(realFile, []byte("original"), 0o644))
	require.NoError(t, os.Symlink("manifest.yaml", linkPath))

	require.NoError(t, AtomicWriteFile(linkPath, []byte("updated")))

	// The real file must contain the new content.
	got, err := os.ReadFile(realFile)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(got))

	// The link must survive with its relative spelling intact.
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "manifest.yaml", target)
}

// A dangling symlink (target missing) cannot be resolved, so the write falls
// back to replacing the link itself with a regular file: the "dangling paths
// are written as-is" half of the AtomicWriteFile contract. Pinning it matters
// because resolving the parent chain and creating the missing target instead
// would be a defensible alternative that silently changes what a write does
// to a broken link.
func TestAtomicWriteFile_DanglingSymlinkIsReplaced(t *testing.T) {
	dir := t.TempDir()

	linkPath := filepath.Join(dir, ".datarobot.yaml")
	missingTarget := filepath.Join(dir, "gone.yaml")

	require.NoError(t, os.Symlink(missingTarget, linkPath))

	require.NoError(t, AtomicWriteFile(linkPath, []byte("payload")))

	// The link is gone, replaced by a regular file holding the payload.
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0), fi.Mode()&os.ModeSymlink, "dangling link should have been replaced, got mode %v", fi.Mode())

	got, err := os.ReadFile(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))

	// The missing target must not have been created as a side effect.
	assert.NoFileExists(t, missingTarget)
}

// A failure after resolution is reported against the path the caller passed:
// the resolved target means nothing to someone who handed over a symlink.
// Pointing the link at a directory makes the rename over it fail without any
// permission-bit setup.
func TestAtomicWriteFile_ErrorNamesTheSymlinkPath(t *testing.T) {
	realDir := t.TempDir()
	linkDir := t.TempDir()

	linkPath := filepath.Join(linkDir, ".datarobot.yaml")
	require.NoError(t, os.Symlink(realDir, linkPath))

	writeErr := AtomicWriteFile(linkPath, []byte("payload"))
	require.Error(t, writeErr)

	// The caller's path leads the message.
	assert.Contains(t, writeErr.Error(), linkPath)

	// The resolved target stays in the chain for debugging. EvalSymlinks
	// resolves the whole chain, including symlinked intermediates such as
	// /tmp on macOS, so compare against the resolved form of realDir.
	resolvedDir, err := filepath.EvalSymlinks(realDir)
	require.NoError(t, err)
	assert.Contains(t, writeErr.Error(), resolvedDir)
}

// TestAtomicWriteFile_CleansTmpWhenRenameFails exercises the post-CreateTemp
// failure path: we pre-seed the target path as a directory so os.Rename
// fails, and then assert the temp file was cleaned up by the defer.
func TestAtomicWriteFile_CleansTmpWhenRenameFails(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.json")

	err := os.Mkdir(target, 0o755)
	require.NoError(t, err)

	err = AtomicWriteFile(target, []byte("payload"))
	require.Error(t, err)

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContainsf(t, e.Name(), ".tmp.",
			"found leftover temp file: %s", e.Name())
	}
}
