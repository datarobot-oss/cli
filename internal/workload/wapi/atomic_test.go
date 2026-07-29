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

package wapi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriteFile_CreatesNew(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "new.json")

	err := atomicWriteFile(target, []byte(`{"hello":"world"}`))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.JSONEq(t, `{"hello":"world"}`, string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)

	if runtime.GOOS != "windows" {
		// Windows does not enforce Unix permission bits.
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
}

func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "existing.json")

	err := os.WriteFile(target, []byte("old"), 0o644)
	require.NoError(t, err)

	err = atomicWriteFile(target, []byte("new"))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// TestAtomicWriteFile_CleansTmpWhenRenameFails exercises the post-CreateTemp
// failure path: we pre-seed the target path as a directory so os.Rename
// fails, and then assert the temp file was cleaned up by the defer.
func TestAtomicWriteFile_CleansTmpWhenRenameFails(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.json")

	err := os.Mkdir(target, 0o755)
	require.NoError(t, err)

	err = atomicWriteFile(target, []byte("payload"))
	require.Error(t, err)

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContainsf(t, e.Name(), ".tmp.",
			"found leftover temp file: %s", e.Name())
	}
}
