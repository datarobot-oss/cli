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

// seedLegacyDir lays down a legacy root state directory with one recognisable
// file in it, so tests can prove the contents travelled rather than just the
// directory name.
func seedLegacyDir(t *testing.T, projectDir string) string {
	t.Helper()

	legacy := filepath.Join(projectDir, LegacyDirName)
	require.NoError(t, os.Mkdir(legacy, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, configFile), []byte(`{"artifactId":"art-1"}`), 0o600))

	return legacy
}

func TestEnsureMigrated_NothingToDo(t *testing.T) {
	tmp := t.TempDir()

	EnsureMigrated(tmp)

	assert.NoDirExists(t, filepath.Join(tmp, RootDirName), "an unlinked project is not given directories it never had")
}

func TestEnsureMigrated_MovesLegacyDir(t *testing.T) {
	tmp := t.TempDir()
	legacy := seedLegacyDir(t, tmp)

	EnsureMigrated(tmp)

	assert.NoDirExists(t, legacy)

	current := filepath.Join(tmp, RootDirName, StateDirName)
	assert.DirExists(t, current)

	// The files travelled, not just the directory.
	raw, err := os.ReadFile(filepath.Join(current, configFile))
	require.NoError(t, err)
	assert.JSONEq(t, `{"artifactId":"art-1"}`, string(raw))

	// Dir now resolves to the new location, and a second call changes nothing.
	assert.Equal(t, current, Dir(tmp))

	EnsureMigrated(tmp)
	assert.Equal(t, current, Dir(tmp))
	assert.NoDirExists(t, legacy)
}

// A project that already keeps state in .datarobot but still has a leftover
// root directory is not merged: the current location wins and the leftover is
// left alone, because guessing which tree is authoritative could destroy state.
func TestEnsureMigrated_BothPresentKeepsCurrent(t *testing.T) {
	tmp := t.TempDir()
	legacy := seedLegacyDir(t, tmp)

	current := filepath.Join(tmp, RootDirName, StateDirName)
	require.NoError(t, os.MkdirAll(current, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(current, configFile), []byte(`{"artifactId":"art-2"}`), 0o600))

	EnsureMigrated(tmp)

	assert.DirExists(t, legacy, "the leftover is left for the user to inspect, not deleted")

	raw, err := os.ReadFile(filepath.Join(current, configFile))
	require.NoError(t, err)
	assert.JSONEq(t, `{"artifactId":"art-2"}`, string(raw), "current state must not be overwritten by the leftover")
}

// A move that cannot happen must not fail the command: the legacy directory
// stays readable and every path helper keeps resolving to it.
func TestEnsureMigrated_UnmovableFallsBackToLegacy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not block rename the same way on Windows")
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}

	tmp := t.TempDir()
	legacy := seedLegacyDir(t, tmp)

	// A read-only project dir blocks both the mkdir of .datarobot and the
	// rename out of the legacy location.
	require.NoError(t, os.Chmod(tmp, 0o500))
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o700) })

	EnsureMigrated(tmp)

	assert.DirExists(t, legacy)
	assert.Equal(t, legacy, Dir(tmp), "commands keep reading the legacy location for this run")
	assert.True(t, Exists(tmp), "the project is still linked")
}
