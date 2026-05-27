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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_ValidPointers(t *testing.T) {
	catalog := "cat-xyz-789"
	version := "fedcba0987654321fedcba09"

	cfg := Config{
		ArtifactID:          "art-abc-123",
		CatalogID:           &catalog,
		LastSyncedVersionID: &version,
		CreatedAt:           time.Date(2026, 4, 10, 9, 15, 0, 0, time.UTC),
		CLIVersion:          "0.2.0",
	}

	require.NoError(t, validateConfig(cfg))

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var decoded Config

	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, validateConfig(decoded))
}

func TestValidateDRNonemptyPtr_OnPointer(t *testing.T) {
	catalog := "cat-xyz-789"

	err := getValidator().Var(&catalog, "dr_nonempty_ptr")
	require.NoError(t, err)
}

func TestIsValidDRID(t *testing.T) {
	assert.True(t, isValidDRID("art-abc-123"))
	assert.False(t, isValidDRID(""))
	assert.False(t, isValidDRID("has/slash"))
	assert.False(t, isValidDRID("has..dots"))
}

func TestIsSHA256Hex(t *testing.T) {
	assert.True(t, isSHA256Hex(testHash('a')))
	assert.False(t, isSHA256Hex(""))
	assert.False(t, isSHA256Hex("abc"))
	assert.False(t, isSHA256Hex(strings.ToUpper(testHash('a'))))
	assert.False(t, isSHA256Hex("0x"+strings.Repeat("a", 62)))
	assert.False(t, isSHA256Hex(strings.Repeat("g", 64)))
}

func TestValidateInitOptions(t *testing.T) {
	version := "fedcba0987654321fedcba09"

	t.Run("valid", func(t *testing.T) {
		tests := []InitOptions{
			{ArtifactID: "art-abc-123"},
			{ArtifactID: "art-abc-123", CatalogID: "cat-xyz-789"},
			{ArtifactID: "art-abc-123", CatalogID: "cat-xyz-789", LastSyncedVersionID: version},
		}

		for _, opts := range tests {
			require.NoError(t, validateInitOptions(opts))
		}
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name    string
			opts    InitOptions
			wantErr string
		}{
			{
				name:    "missing artifactId",
				opts:    InitOptions{},
				wantErr: "artifactId is required",
			},
			{
				name:    "lastSyncedVersionId without catalogId",
				opts:    InitOptions{ArtifactID: "art-abc-123", LastSyncedVersionID: version},
				wantErr: "catalogId is required when lastSyncedVersionId is set",
			},
			{
				name:    "artifactId with path separator",
				opts:    InitOptions{ArtifactID: "art/evil"},
				wantErr: "artifactId must be a non-empty identifier without path separators",
			},
			{
				name:    "catalogId with path separator",
				opts:    InitOptions{ArtifactID: "art-abc-123", CatalogID: `cat\evil`},
				wantErr: "catalogId must be a non-empty identifier without path separators",
			},
			{
				name: "lastSyncedVersionId with path separator",
				opts: InitOptions{
					ArtifactID:          "art-abc-123",
					CatalogID:           "cat-xyz-789",
					LastSyncedVersionID: "../escape",
				},
				wantErr: "lastSyncedVersionId must be a non-empty identifier without path separators",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := validateInitOptions(tc.opts)
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	})
}

func TestValidateConfig(t *testing.T) {
	createdAt := time.Date(2026, 4, 10, 9, 15, 0, 0, time.UTC)
	version := "fedcba0987654321fedcba09"

	t.Run("valid nil optionals", func(t *testing.T) {
		cfg := Config{
			ArtifactID: "art-abc-123",
			CreatedAt:  createdAt,
			CLIVersion: "0.2.0",
		}

		require.NoError(t, validateConfig(cfg))
	})

	t.Run("errors", func(t *testing.T) {
		empty := ""

		tests := []struct {
			name    string
			cfg     Config
			wantErr string
		}{
			{
				name: "missing artifactId",
				cfg: Config{
					CreatedAt:  createdAt,
					CLIVersion: "0.2.0",
				},
				wantErr: "artifactId is required",
			},
			{
				name: "empty catalogId pointer",
				cfg: Config{
					ArtifactID: "art-abc-123",
					CatalogID:  &empty,
					CreatedAt:  createdAt,
					CLIVersion: "0.2.0",
				},
				wantErr: "catalogId must not be empty when set",
			},
			{
				name: "empty lastSyncedVersionId pointer",
				cfg: Config{
					ArtifactID:          "art-abc-123",
					LastSyncedVersionID: &empty,
					CreatedAt:           createdAt,
					CLIVersion:          "0.2.0",
				},
				wantErr: "lastSyncedVersionId must not be empty when set",
			},
			{
				name: "zero createdAt",
				cfg: Config{
					ArtifactID: "art-abc-123",
					CLIVersion: "0.2.0",
				},
				wantErr: "createdAt is required",
			},
			{
				name: "missing cliVersion",
				cfg: Config{
					ArtifactID: "art-abc-123",
					CreatedAt:  createdAt,
				},
				wantErr: "cliVersion is required",
			},
			{
				name: "artifactId with path separator",
				cfg: Config{
					ArtifactID: "../art",
					CreatedAt:  createdAt,
					CLIVersion: "0.2.0",
				},
				wantErr: "artifactId must be a non-empty identifier without path separators",
			},
			{
				name: "lastSyncedVersionId without catalogId",
				cfg: Config{
					ArtifactID:          "art-abc-123",
					LastSyncedVersionID: &version,
					CreatedAt:           createdAt,
					CLIVersion:          "0.2.0",
				},
				wantErr: "catalogId is required when lastSyncedVersionId is set",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := validateConfig(tc.cfg)
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	})
}

func TestValidateManifest(t *testing.T) {
	synced := time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC)
	versionID := "fedcba0987654321fedcba09"
	validHash := testHash('a')

	t.Run("valid empty base", func(t *testing.T) {
		require.NoError(t, validateManifest(Manifest{Version: ManifestVersion, Files: map[string]FileMeta{}}))
	})

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name     string
			manifest Manifest
			wantErr  string
		}{
			{
				name:     "unsupported version",
				manifest: Manifest{Version: 2, Files: map[string]FileMeta{}},
				wantErr:  "version must equal 1",
			},
			{
				name: "syncedAt without syncedVersionId",
				manifest: Manifest{
					Version:  ManifestVersion,
					SyncedAt: &synced,
					Files:    map[string]FileMeta{},
				},
				wantErr: "syncedVersionId is required when syncedAt is set",
			},
			{
				name: "syncedVersionId without syncedAt",
				manifest: Manifest{
					Version:         ManifestVersion,
					SyncedVersionID: &versionID,
					Files:           map[string]FileMeta{},
				},
				wantErr: "syncedAt is required when syncedVersionId is set",
			},
			{
				name: "path escapes root",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"../escape.py": {Hash: validHash, Size: 1},
					},
				},
				wantErr: "path escapes project root",
			},
			{
				name: "hash too short",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"a.py": {Hash: "abc", Size: 1},
					},
				},
				wantErr: "hash must be a 64-character lowercase SHA-256 hex string",
			},
			{
				name: "hash not hex",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"a.py": {Hash: strings.Repeat("g", 64), Size: 1},
					},
				},
				wantErr: "hash must be a 64-character lowercase SHA-256 hex string",
			},
			{
				name: "hash uppercase",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"a.py": {Hash: strings.ToUpper(validHash), Size: 1},
					},
				},
				wantErr: "hash must be a 64-character lowercase SHA-256 hex string",
			},
			{
				name: "hash with 0x prefix",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"a.py": {Hash: "0x" + strings.Repeat("a", 62), Size: 1},
					},
				},
				wantErr: "hash must be a 64-character lowercase SHA-256 hex string",
			},
			{
				name: "negative size",
				manifest: Manifest{
					Version: ManifestVersion,
					Files: map[string]FileMeta{
						"a.py": {Hash: validHash, Size: -1},
					},
				},
				wantErr: "size must be >= 0",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := validateManifest(tc.manifest)
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
			})
		}
	})
}
