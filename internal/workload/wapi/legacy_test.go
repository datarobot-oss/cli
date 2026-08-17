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

// seedLegacy writes the state layout an older CLI produced: a root .wapi/
// holding config.json, and nothing under .datarobot/.
func seedLegacy(t *testing.T, body string) string {
	t.Helper()

	tmp := t.TempDir()
	dir := filepath.Join(tmp, LegacyDirName)

	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600))

	return tmp
}

// The deliberate breaking change of this release: state at the old location no
// longer makes a project linked. Pinned so that re-adding a fallback is a
// visible decision rather than something that slips back in unnoticed.
func TestLegacyStateIsNotLinked(t *testing.T) {
	tmp := seedLegacy(t, `{"artifactId":"art-abc-123"}`)

	assert.False(t, Exists(tmp), "state at the old location does not make a project linked")
	assert.Equal(t, filepath.Join(tmp, RootDirName, StateDirName), Dir(tmp),
		"Dir never resolves to the old location")
}

func TestLegacyState_FoundWithArtifactID(t *testing.T) {
	tmp := seedLegacy(t, `{"artifactId":"art-abc-123"}`)

	id, found := LegacyState(tmp)
	assert.True(t, found)
	assert.Equal(t, "art-abc-123", id)
}

// A config that will not parse still counts as found: the directory being
// there is what changes the advice, and this user needs more help, not less.
func TestLegacyState_FoundWithUnreadableConfig(t *testing.T) {
	tmp := seedLegacy(t, "{ this is not json")

	id, found := LegacyState(tmp)
	assert.True(t, found)
	assert.Empty(t, id)
}

func TestLegacyState_AbsentOnCleanProject(t *testing.T) {
	tmp := t.TempDir()

	id, found := LegacyState(tmp)
	assert.False(t, found)
	assert.Empty(t, id)
}

// The whole point of the guard: the error has to name the directory and the id,
// because the directory is gitignored and the id is the only way back.
func TestNotLinkedError_NamesLegacyDirAndArtifact(t *testing.T) {
	tmp := seedLegacy(t, `{"artifactId":"art-abc-123"}`)

	err := NotLinkedError(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked", "callers and tests match on this prefix")
	assert.Contains(t, err.Error(), LegacyDirName)
	assert.Contains(t, err.Error(), "art-abc-123")
}

func TestNotLinkedError_FallsBackToPlaceholderWithoutID(t *testing.T) {
	tmp := seedLegacy(t, "{ this is not json")

	err := NotLinkedError(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), LegacyDirName)
	assert.Contains(t, err.Error(), "<artifact-id>")
}

func TestNotLinkedError_PlainHintOnCleanProject(t *testing.T) {
	err := NotLinkedError(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
	assert.NotContains(t, err.Error(), LegacyDirName,
		"a project that was never linked should not hear about a directory it does not have")
}

func TestLegacyStateNotice(t *testing.T) {
	tmp := seedLegacy(t, `{"artifactId":"art-abc-123"}`)

	assert.Contains(t, LegacyStateNotice(tmp), LegacyDirName)
	assert.Empty(t, LegacyStateNotice(t.TempDir()),
		"empty so callers can pass the result straight to format.StateNotice")
}
