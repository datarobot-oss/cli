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
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExists_HappyPath(t *testing.T) {
	tmp := t.TempDir()

	file := filepath.Join(tmp, "f")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	dir := filepath.Join(tmp, "d")
	require.NoError(t, os.Mkdir(dir, 0o755))

	assert.True(t, FileExists(file))
	assert.False(t, DirExists(file))

	assert.True(t, DirExists(dir))
	assert.False(t, FileExists(dir))

	missing := filepath.Join(tmp, "nope")
	assert.False(t, FileExists(missing))
	assert.False(t, DirExists(missing))
}

// A path whose parent is a regular file stats as ENOTDIR, which is not
// ErrNotExist and yields no FileInfo. Both helpers must report false rather
// than dereference the nil.
func TestExists_ParentIsAFile(t *testing.T) {
	tmp := t.TempDir()

	parent := filepath.Join(tmp, "parent")
	require.NoError(t, os.WriteFile(parent, []byte("not a directory"), 0o600))

	nested := filepath.Join(parent, "child")

	_, err := os.Stat(nested)
	require.Error(t, err)
	require.False(t, os.IsNotExist(err), "this test is meaningless if ENOTDIR reports as not-exist")

	assert.NotPanics(t, func() {
		assert.False(t, DirExists(nested))
		assert.False(t, FileExists(nested))
	})
}

// An unreadable parent directory stats as EACCES, the other error class that
// returns no FileInfo.
func TestExists_UnreadableParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not gate stat the same way on Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}

	tmp := t.TempDir()

	parent := filepath.Join(tmp, "parent")
	require.NoError(t, os.Mkdir(parent, 0o755))
	require.NoError(t, os.Chmod(parent, 0o000))

	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	nested := filepath.Join(parent, "child")

	assert.NotPanics(t, func() {
		assert.False(t, DirExists(nested))
		assert.False(t, FileExists(nested))
	})
}
