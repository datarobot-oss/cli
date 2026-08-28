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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExists_Missing(t *testing.T) {
	tmp := t.TempDir()

	assert.False(t, Exists(tmp))
}

func TestExists_PresentDir(t *testing.T) {
	tmp := t.TempDir()

	err := os.MkdirAll(filepath.Join(tmp, RootDirName, StateDirName), 0o755)
	require.NoError(t, err)

	assert.True(t, Exists(tmp))
}

// A project linked by an older CLI is still linked: Dir falls back to the
// legacy location until EnsureMigrated moves it.
func TestExists_PresentAtLegacyDir(t *testing.T) {
	tmp := t.TempDir()

	err := os.Mkdir(filepath.Join(tmp, LegacyDirName), 0o755)
	require.NoError(t, err)

	assert.True(t, Exists(tmp))
	assert.Equal(t, filepath.Join(tmp, LegacyDirName), Dir(tmp))
}

// The current location wins when both exist, so a half-finished migration
// cannot make commands read stale state.
func TestDir_PrefersCurrentOverLegacy(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, RootDirName, StateDirName), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(tmp, LegacyDirName), 0o755))

	assert.Equal(t, filepath.Join(tmp, RootDirName, StateDirName), Dir(tmp))
}

// With nothing on disk yet, Dir names where state will be created rather
// than the legacy location.
func TestDir_UnlinkedProjectNamesCurrentLocation(t *testing.T) {
	tmp := t.TempDir()

	assert.Equal(t, filepath.Join(tmp, RootDirName, StateDirName), Dir(tmp))
}

func TestExists_IsFileNotDir(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(tmp, RootDirName), 0o755))

	err := os.WriteFile(filepath.Join(tmp, RootDirName, StateDirName), []byte("oops"), 0o644)
	require.NoError(t, err)

	assert.False(t, Exists(tmp))
}
