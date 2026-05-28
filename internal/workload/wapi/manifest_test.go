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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest_SaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	synced := time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC)
	versionID := "fedcba0987654321fedcba09"

	original := Manifest{
		Version:         ManifestVersion,
		SyncedAt:        &synced,
		SyncedVersionID: &versionID,
		Files: map[string]FileMeta{
			"agent.py":                {Hash: testHash('1'), Size: 1234},
			"utils/helper.py":         {Hash: testHash('a'), Size: 567},
			"models/bert/weights.bin": {Hash: testHash('d'), Size: 45678912},
		},
	}

	err := SaveManifest(tmp, original)
	require.NoError(t, err)

	got, err := LoadManifest(tmp)
	require.NoError(t, err)

	assert.Equal(t, original.Version, got.Version)
	require.NotNil(t, got.SyncedAt)
	assert.True(t, synced.Equal(*got.SyncedAt))
	require.NotNil(t, got.SyncedVersionID)
	assert.Equal(t, versionID, *got.SyncedVersionID)
	assert.Equal(t, original.Files, got.Files)
}

func TestManifest_EmptyFilesMap(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	err := SaveManifest(tmp, Manifest{Version: ManifestVersion})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(tmp, DirName, manifestFile))
	require.NoError(t, err)

	var parsed map[string]any

	err = json.Unmarshal(raw, &parsed)
	require.NoError(t, err)

	assert.Nil(t, parsed["syncedAt"])
	assert.Nil(t, parsed["syncedVersionId"])

	files, ok := parsed["files"].(map[string]any)
	require.True(t, ok, "files should serialize as object, not null")
	assert.Empty(t, files)

	got, err := LoadManifest(tmp)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Version)
	assert.Empty(t, got.Files)
	assert.NotNil(t, got.Files, "LoadManifest should normalize nil Files to empty map")
}

func TestManifest_PreservesPathsVerbatim(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	paths := map[string]FileMeta{
		"forward/slash.py":   {Hash: testHash('a'), Size: 1},
		"deep/nested/x.py":   {Hash: testHash('b'), Size: 2},
		"unicode/café.py":    {Hash: testHash('c'), Size: 3},
		"with spaces/f.json": {Hash: testHash('d'), Size: 4},
	}

	err := SaveManifest(tmp, Manifest{Version: ManifestVersion, Files: paths})
	require.NoError(t, err)

	got, err := LoadManifest(tmp)
	require.NoError(t, err)
	assert.Equal(t, paths, got.Files)
}

func TestManifest_LoadNotInitialized(t *testing.T) {
	tmp := t.TempDir()

	_, err := LoadManifest(tmp)
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestManifest_SaveNotInitialized(t *testing.T) {
	tmp := t.TempDir()

	err := SaveManifest(tmp, Manifest{Version: ManifestVersion})
	assert.ErrorIs(t, err, ErrNotInitialized)
}

func TestManifest_LoadCorrupted(t *testing.T) {
	tmp := t.TempDir()
	initWapiDir(t, tmp)

	path := filepath.Join(tmp, DirName, manifestFile)
	err := os.WriteFile(path, []byte("not json"), 0o644)
	require.NoError(t, err)

	_, err = LoadManifest(tmp)
	require.Error(t, err)

	var ce *CorruptedError

	require.ErrorAs(t, err, &ce)
	assert.Equal(t, path, ce.Path)
}

func TestManifest_LoadInvalid(t *testing.T) {
	synced := "2026-04-10T09:30:00Z"
	versionID := "fedcba0987654321fedcba09"
	validHash := testHash('a')

	tests := []struct {
		name string
		json string
	}{
		{
			name: "unsupported version",
			json: `{"version":2,"files":{}}`,
		},
		{
			name: "path escapes root",
			json: `{"version":1,"files":{"../escape.py":{"hash":"` + validHash + `","size":1}}}`,
		},
		{
			name: "hash too short",
			json: `{"version":1,"files":{"a.py":{"hash":"abc","size":1}}}`,
		},
		{
			name: "hash not hex",
			json: `{"version":1,"files":{"a.py":{"hash":"` + strings.Repeat("g", 64) + `","size":1}}}`,
		},
		{
			name: "negative size",
			json: `{"version":1,"files":{"a.py":{"hash":"` + validHash + `","size":-1}}}`,
		},
		{
			name: "syncedAt without syncedVersionId",
			json: `{"version":1,"syncedAt":"` + synced + `","files":{}}`,
		},
		{
			name: "syncedVersionId without syncedAt",
			json: `{"version":1,"syncedVersionId":"` + versionID + `","files":{}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			initWapiDir(t, tmp)

			path := filepath.Join(tmp, DirName, manifestFile)
			err := os.WriteFile(path, []byte(tc.json), 0o644)
			require.NoError(t, err)

			_, err = LoadManifest(tmp)
			require.Error(t, err)

			var ce *CorruptedError

			require.ErrorAs(t, err, &ce)
			assert.Equal(t, path, ce.Path)
		})
	}
}
