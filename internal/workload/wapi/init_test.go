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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialize_CreatesFullLayout(t *testing.T) {
	tmp := t.TempDir()

	before := time.Now().UTC()
	err := Initialize(tmp, InitOptions{
		ArtifactID:          "art-abc-123",
		CatalogID:           "cat-xyz-789",
		LastSyncedVersionID: "fedcba0987654321fedcba09",
	})
	after := time.Now().UTC()

	require.NoError(t, err)

	// State dir listing — exactly the four machine-managed files.
	entries, err := os.ReadDir(Dir(tmp))
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.ElementsMatch(t, []string{gitignoreFile, configFile, HistoryFile, manifestFile}, names)

	// The self-ignoring .gitignore is "*\n".
	gi, err := os.ReadFile(filepath.Join(Dir(tmp), gitignoreFile))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gi))

	// .drignore at project root matches the embedded template.
	rootIgnore, err := os.ReadFile(ignorePath(tmp))
	require.NoError(t, err)
	assert.Equal(t, ignoreTemplate, rootIgnore)

	assert.NoFileExists(t, filepath.Join(tmp, ignore.LegacyFileName), "init writes the current name only")

	// config.json round-trips cleanly.
	cfg, err := LoadConfig(tmp)
	require.NoError(t, err)
	assert.Equal(t, "art-abc-123", cfg.ArtifactID)
	require.NotNil(t, cfg.CatalogID)
	assert.Equal(t, "cat-xyz-789", *cfg.CatalogID)
	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, "fedcba0987654321fedcba09", *cfg.LastSyncedVersionID)
	assert.Equal(t, version.Version, cfg.CLIVersion)
	assert.False(t, cfg.CreatedAt.Before(before), "CreatedAt should be >= before")
	assert.False(t, cfg.CreatedAt.After(after), "CreatedAt should be <= after")

	// manifest.json is empty BASE with explicit null sync pointers.
	raw, err := os.ReadFile(filepath.Join(Dir(tmp), manifestFile))
	require.NoError(t, err)

	var parsed map[string]any

	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)
	assert.EqualValues(t, 1, parsed["version"])
	assert.Nil(t, parsed["syncedAt"])
	assert.Nil(t, parsed["syncedVersionId"])

	files, ok := parsed["files"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, files)
}

func TestInitialize_WritesInitHistoryEntry(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc-123"})
	require.NoError(t, err)

	entries := readHistoryLines(t, filepath.Join(Dir(tmp), HistoryFile))
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "init", e["op"])
	assert.Equal(t, "art-abc-123", e["artifact"])
	assert.EqualValues(t, 0, e["baseFiles"])
	assert.NotEmpty(t, e["ts"])
	assert.Nil(t, e["catalog"], "empty-artifact init must record catalog as JSON null")

	_, err = time.Parse(time.RFC3339, e["ts"].(string))
	assert.NoError(t, err, "ts should parse as RFC3339")
}

func TestInitialize_HistoryCatalogPresentForExistingCode(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{
		ArtifactID: "art-abc-123",
		CatalogID:  "cat-xyz-789",
	})
	require.NoError(t, err)

	entries := readHistoryLines(t, filepath.Join(Dir(tmp), HistoryFile))
	require.Len(t, entries, 1)
	assert.Equal(t, "cat-xyz-789", entries[0]["catalog"])
}

func TestInitialize_CreatesMissingProjectDir(t *testing.T) {
	parent := t.TempDir()
	nested := filepath.Join(parent, "a", "b", "c")

	err := Initialize(nested, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	assert.True(t, Exists(nested), "project dir + state dir must be created end-to-end")
}

func TestInitialize_AlreadyLinked(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	err = Initialize(tmp, InitOptions{ArtifactID: "art-def"})
	assert.ErrorIs(t, err, ErrAlreadyLinked)
}

func TestInitialize_PreservesUserIgnoreFile(t *testing.T) {
	tmp := t.TempDir()

	custom := []byte("# my custom ignore\nsecret/\n")
	err := os.WriteFile(ignorePath(tmp), custom, 0o644)
	require.NoError(t, err)

	err = Initialize(tmp, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	got, err := os.ReadFile(ignorePath(tmp))
	require.NoError(t, err)
	assert.Equal(t, custom, got, "existing .drignore must not be overwritten")
}

// A project still on the legacy name already has ignore patterns, and the
// current name wins when both exist. Dropping the default template beside a
// tuned legacy file would leave every byte of the user's file intact while
// quietly retiring every pattern in it.
func TestInitialize_LeavesLegacyIgnoreFileAlone(t *testing.T) {
	tmp := t.TempDir()

	legacy := filepath.Join(tmp, ignore.LegacyFileName)

	custom := []byte("# my custom ignore\nsecret/\n")
	err := os.WriteFile(legacy, custom, 0o644)
	require.NoError(t, err)

	err = Initialize(tmp, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	assert.NoFileExists(t, ignorePath(tmp), "a legacy file counts as already having one")

	got, err := os.ReadFile(legacy)
	require.NoError(t, err)
	assert.Equal(t, custom, got)
}

func TestInitialize_NullsForEmptyOptionals(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(Dir(tmp), configFile))
	require.NoError(t, err)

	var parsed map[string]any

	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)
	assert.Nil(t, parsed["catalogId"])
	assert.Nil(t, parsed["lastSyncedVersionId"])
}

func TestInitialize_WritesCLIVersion(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc"})
	require.NoError(t, err)

	cfg, err := LoadConfig(tmp)
	require.NoError(t, err)
	assert.Equal(t, version.Version, cfg.CLIVersion)
}

func TestInitialize_InvalidOptions(t *testing.T) {
	t.Run("empty artifact ID", func(t *testing.T) {
		tmp := t.TempDir()

		err := Initialize(tmp, InitOptions{ArtifactID: ""})
		require.Error(t, err)
		assert.False(t, Exists(tmp))
	})

	t.Run("lastSyncedVersionId without catalogId", func(t *testing.T) {
		tmp := t.TempDir()

		err := Initialize(tmp, InitOptions{
			ArtifactID:          "art-abc-123",
			LastSyncedVersionID: "fedcba0987654321fedcba09",
		})
		require.Error(t, err)
		assert.False(t, Exists(tmp))
	})
}
