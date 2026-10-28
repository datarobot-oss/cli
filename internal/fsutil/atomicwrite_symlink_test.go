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

package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireSymlinks skips the test when the process cannot create a symlink in a
// temp directory. Windows needs SeCreateSymbolicLinkPrivilege or Developer
// Mode; these tests are not exercised in Windows CI (the runners lack the
// privilege), but they run on POSIX CI and on privileged or Developer-Mode
// Windows machines.
func requireSymlinks(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	link := filepath.Join(dir, "probe")
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o644))

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}
}

// When path is a symlink, the write must land in the real file and leave the
// link intact. Symlink creation needs elevated privileges on Windows, so
// requireSymlinks gates the test (see its comment for where it runs).
func TestAtomicWriteFile_WriteThroughSymlink(t *testing.T) {
	requireSymlinks(t)

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
	requireSymlinks(t)

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

// A chain of symlinks (link → link → real file) must resolve through every
// hop so the write lands in the real file and every link in the chain
// survives. EvalSymlinks follows the full chain; this test pins that behavior
// so a future refactor that short-circuits after the first hop is caught.
func TestAtomicWriteFile_WriteThroughSymlinkChain(t *testing.T) {
	requireSymlinks(t)

	dir := t.TempDir()

	realFile := filepath.Join(dir, "manifest.yaml")
	link1 := filepath.Join(dir, "link1.yaml")
	link2 := filepath.Join(dir, "link2.yaml")

	require.NoError(t, os.WriteFile(realFile, []byte("original"), 0o644))
	require.NoError(t, os.Symlink(realFile, link1))
	require.NoError(t, os.Symlink(link1, link2))

	require.NoError(t, AtomicWriteFile(link2, []byte("updated")))

	// The real file must contain the new content.
	got, err := os.ReadFile(realFile)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(got))

	// Both links must survive as symlinks.
	for _, link := range []string{link1, link2} {
		fi, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotEqual(t, os.FileMode(0), fi.Mode()&os.ModeSymlink,
			"%s should still be a symlink, got mode %v", link, fi.Mode())
	}
}

// A self-referential (circular) symlink cannot be resolved, and the write
// must fail fast: resolveWriteTarget returns the resolve error before the
// write begins, so the symlink is never at risk of being replaced by a
// regular file. The error names the caller's path with "resolve symlink"
// context so a caller who passed a symlink knows where to look.
func TestAtomicWriteFile_CircularSymlinkFailsSafely(t *testing.T) {
	requireSymlinks(t)

	dir := t.TempDir()

	linkPath := filepath.Join(dir, "circular.yaml")
	require.NoError(t, os.Symlink(linkPath, linkPath))

	writeErr := AtomicWriteFile(linkPath, []byte("payload"))
	require.Error(t, writeErr)

	// The symlink must survive — the write must not have reached rename.
	fi, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0), fi.Mode()&os.ModeSymlink,
		"circular link should not have been replaced, got mode %v", fi.Mode())

	// The error should carry "resolve symlink" context naming the caller's
	// path, since resolveWriteTarget fails fast before the write begins.
	assert.Contains(t, writeErr.Error(), "resolve symlink")
	assert.Contains(t, writeErr.Error(), linkPath)
}

// A dangling symlink (target missing) cannot be resolved, so the write falls
// back to replacing the link itself with a regular file: the "dangling paths
// are written as-is" half of the AtomicWriteFile contract. Pinning it matters
// because resolving the parent chain and creating the missing target instead
// would be a defensible alternative that silently changes what a write does
// to a broken link.
func TestAtomicWriteFile_DanglingSymlinkIsReplaced(t *testing.T) {
	requireSymlinks(t)

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
	requireSymlinks(t)

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
