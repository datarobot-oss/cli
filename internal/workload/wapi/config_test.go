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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_SaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	catalog := "cat-xyz-789"
	version := "fedcba0987654321fedcba09"

	original := Config{
		ArtifactID:          "art-abc-123",
		CatalogID:           &catalog,
		LastSyncedVersionID: &version,
		CreatedAt:           time.Date(2026, 4, 10, 9, 15, 0, 0, time.UTC),
		CLIVersion:          "0.2.0",
	}

	err := SaveConfig(tmp, original)
	require.NoError(t, err)

	got, err := LoadConfig(tmp)
	require.NoError(t, err)

	assert.Equal(t, original.ArtifactID, got.ArtifactID)
	assert.Equal(t, original.CLIVersion, got.CLIVersion)
	require.NotNil(t, got.CatalogID)
	assert.Equal(t, catalog, *got.CatalogID)
	require.NotNil(t, got.LastSyncedVersionID)
	assert.Equal(t, version, *got.LastSyncedVersionID)
	assert.True(t, original.CreatedAt.Equal(got.CreatedAt))
}

func TestConfig_SaveLoadRoundTrip_NullsForEmptyOptionals(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	err := SaveConfig(tmp, Config{
		ArtifactID:          "art-abc-123",
		CatalogID:           nil,
		LastSyncedVersionID: nil,
		CreatedAt:           time.Date(2026, 4, 10, 9, 15, 0, 0, time.UTC),
		CLIVersion:          "0.2.0",
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(tmp, DirName, ConfigFile))
	require.NoError(t, err)

	var parsed map[string]any

	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	assert.Nil(t, parsed["catalogId"])
	assert.Nil(t, parsed["lastSyncedVersionId"])

	// Round-trip still returns nil pointers.
	got, err := LoadConfig(tmp)
	require.NoError(t, err)
	assert.Nil(t, got.CatalogID)
	assert.Nil(t, got.LastSyncedVersionID)
}

func TestConfig_LoadNotInitialized(t *testing.T) {
	tmp := t.TempDir()

	_, err := LoadConfig(tmp)
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestConfig_SaveNotInitialized(t *testing.T) {
	tmp := t.TempDir()

	err := SaveConfig(tmp, Config{ArtifactID: "art-abc"})
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestConfig_LoadCorrupted_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, ConfigFile)
	err := os.WriteFile(path, []byte("not json {"), 0o644)
	require.NoError(t, err)

	_, err = LoadConfig(tmp)
	require.Error(t, err)

	var ce *CorruptedError

	require.ErrorAs(t, err, &ce)
	assert.Equal(t, path, ce.Path)
	assert.Error(t, ce.Unwrap())
}

func TestConfig_LoadCorrupted_WrongType(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, ConfigFile)
	err := os.WriteFile(path, []byte(`{"createdAt": 42}`), 0o644)
	require.NoError(t, err)

	_, err = LoadConfig(tmp)
	require.Error(t, err)

	var ce *CorruptedError

	require.ErrorAs(t, err, &ce)
	assert.Equal(t, path, ce.Path)
}
