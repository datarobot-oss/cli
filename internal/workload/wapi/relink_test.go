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
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linked(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	require.NoError(t, Initialize(tmp, InitOptions{
		ArtifactID:          "68b0aaaa0000000000000001",
		CatalogID:           "68b0bbbb0000000000000002",
		LastSyncedVersionID: "68b0cccc0000000000000003",
	}))

	return tmp
}

func TestRelink_PointsTheProjectAtTheNewArtifact(t *testing.T) {
	tmp := linked(t)

	before, err := LoadConfig(tmp)
	require.NoError(t, err)

	require.NoError(t, Relink(tmp, InitOptions{
		ArtifactID: "68b0dddd0000000000000004",
		CatalogID:  "68b0eeee0000000000000005",
	}))

	cfg, err := LoadConfig(tmp)
	require.NoError(t, err)

	assert.Equal(t, "68b0dddd0000000000000004", cfg.ArtifactID)

	// The catalog comes from the artifact being linked to, not from the one
	// being left. Carrying the old one across would have the next sync diff
	// against a code store the new artifact never had.
	require.NotNil(t, cfg.CatalogID)
	assert.Equal(t, "68b0eeee0000000000000005", *cfg.CatalogID)
	assert.Nil(t, cfg.LastSyncedVersionID, "nothing has been synced into the new artifact yet")

	assert.Equal(t, before.CreatedAt, cfg.CreatedAt,
		"CreatedAt records when this became a linked project, which re-pointing does not change")
}

// The BASE records the tree last synced into the artifact being left behind.
func TestRelink_ResetsTheBaseManifest(t *testing.T) {
	tmp := linked(t)

	require.NoError(t, SaveManifest(tmp, Manifest{
		Version: ManifestVersion,
		Files:   map[string]FileMeta{"main.py": {}},
	}))

	require.NoError(t, Relink(tmp, InitOptions{ArtifactID: "68b0dddd0000000000000004"}))

	m, err := LoadManifest(tmp)
	require.NoError(t, err)
	assert.Empty(t, m.Files, "the new artifact has never seen these files")
}

// The whole reason this exists rather than "delete the state directory": the
// things a deletion would take with it survive.
func TestRelink_KeepsTheHistoryAndTheIgnoreFile(t *testing.T) {
	tmp := linked(t)

	ignorePath := ignore.Locate(tmp)
	require.NotEmpty(t, ignorePath, "Initialize drops a starter ignore file")
	require.NoError(t, os.WriteFile(ignorePath, []byte("# mine\n*.tmp\n"), 0o600))

	require.NoError(t, Relink(tmp, InitOptions{ArtifactID: "68b0dddd0000000000000004"}))

	kept, err := os.ReadFile(ignorePath)
	require.NoError(t, err)
	assert.Contains(t, string(kept), "*.tmp", "the user's patterns are not this command's to discard")

	history, err := os.ReadFile(historyPath(tmp))
	require.NoError(t, err)

	text := string(history)
	assert.Contains(t, text, `"op":"init"`, "the log is appended to, not started over")
	assert.Contains(t, text, `"op":"relink"`)
	assert.Contains(t, text, "68b0aaaa0000000000000001", "the id left behind is the way back")
	assert.Contains(t, text, "68b0dddd0000000000000004")
}

// Relinking is not a way to create a link. Initialize is, and it can say what
// a fresh one needs.
func TestRelink_RefusesAnUnlinkedDirectory(t *testing.T) {
	err := Relink(t.TempDir(), InitOptions{ArtifactID: "68b0dddd0000000000000004"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestRelink_RefusesAnArtifactIDItCannotUse(t *testing.T) {
	err := Relink(linked(t), InitOptions{})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "artifactId") ||
		strings.Contains(err.Error(), "ArtifactID"), "name the field that is missing: %v", err)
}

// A config that cannot be read is reported as itself rather than silently
// replaced, because it may not be this project's state at all.
func TestRelink_ReportsACorruptedConfig(t *testing.T) {
	tmp := linked(t)
	require.NoError(t, os.WriteFile(configPath(tmp), []byte("{not json"), 0o600))

	err := Relink(tmp, InitOptions{ArtifactID: "68b0dddd0000000000000004"})
	require.Error(t, err)
}
