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
		Branch:              BranchExistingCode,
	})
	after := time.Now().UTC()

	require.NoError(t, err)

	// .wapi/ listing — exactly the four machine-managed files.
	entries, err := os.ReadDir(filepath.Join(tmp, DirName))
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	assert.ElementsMatch(t, []string{GitignoreFile, ConfigFile, HistoryFile, ManifestFile}, names)

	// .wapi/.gitignore is "*\n".
	gi, err := os.ReadFile(filepath.Join(tmp, DirName, GitignoreFile))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gi))

	// .wapiignore at project root matches the embedded template.
	rootIgnore, err := os.ReadFile(filepath.Join(tmp, WapiignoreFile))
	require.NoError(t, err)
	assert.Equal(t, WapiignoreTemplate, rootIgnore)

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
	raw, err := os.ReadFile(filepath.Join(tmp, DirName, ManifestFile))
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

	err := Initialize(tmp, InitOptions{
		ArtifactID: "art-abc-123",
		Branch:     BranchEmpty,
	})
	require.NoError(t, err)

	entries := readHistoryLines(t, filepath.Join(tmp, DirName, HistoryFile))
	require.Len(t, entries, 1)

	e := entries[0]
	assert.Equal(t, "init", e["op"])
	assert.Equal(t, "art-abc-123", e["artifact"])
	assert.Equal(t, "empty", e["branch"])
	assert.EqualValues(t, 0, e["baseFiles"])
	assert.NotEmpty(t, e["ts"])

	_, err = time.Parse(time.RFC3339, e["ts"].(string))
	assert.NoError(t, err, "ts should parse as RFC3339")
}

func TestInitialize_AlreadyLinked(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc", Branch: BranchEmpty})
	require.NoError(t, err)

	err = Initialize(tmp, InitOptions{ArtifactID: "art-def", Branch: BranchEmpty})
	assert.ErrorIs(t, err, ErrAlreadyLinked)
}

func TestInitialize_PreservesUserWapiignore(t *testing.T) {
	tmp := t.TempDir()

	custom := []byte("# my custom ignore\nsecret/\n")
	err := os.WriteFile(filepath.Join(tmp, WapiignoreFile), custom, 0o644)
	require.NoError(t, err)

	err = Initialize(tmp, InitOptions{ArtifactID: "art-abc", Branch: BranchEmpty})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(tmp, WapiignoreFile))
	require.NoError(t, err)
	assert.Equal(t, custom, got, "existing .wapiignore must not be overwritten")
}

func TestInitialize_NullsForEmptyOptionals(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{
		ArtifactID: "art-abc",
		Branch:     BranchEmpty,
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(tmp, DirName, ConfigFile))
	require.NoError(t, err)

	var parsed map[string]any

	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)
	assert.Nil(t, parsed["catalogId"])
	assert.Nil(t, parsed["lastSyncedVersionId"])
}

func TestInitialize_WritesCLIVersion(t *testing.T) {
	tmp := t.TempDir()

	err := Initialize(tmp, InitOptions{ArtifactID: "art-abc", Branch: BranchEmpty})
	require.NoError(t, err)

	cfg, err := LoadConfig(tmp)
	require.NoError(t, err)
	assert.Equal(t, version.Version, cfg.CLIVersion)
}
