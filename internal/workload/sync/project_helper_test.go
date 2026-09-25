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

package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/require"
)

// syncedProject creates a temp project tree with initialized workload state
// (config.json + manifest.json) where the manifest reflects the current file
// hashes, simulating a project that has already been synced. The config
// points at the given catalogID and versionID, and the manifest's file
// entries match the SHA-256 of every file on disk (user files + .drignore).
//
// This is the reusable helper for tests that need a project in a "synced"
// state: local == base, so a Plan with no disk changes produces an empty plan,
// and a Plan after modifying a file produces exactly that file as an upload.
func syncedProject(t *testing.T, files map[string]string, catalogID, versionID string) string {
	t.Helper()

	dir := initProject(t, files)

	// Update config with catalog and version to simulate a prior sync.
	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	cfg.CatalogID = &catalogID
	cfg.LastSyncedVersionID = &versionID
	require.NoError(t, wapi.SaveConfig(dir, cfg))

	// Build manifest with hashes of ALL files on disk: user files plus the
	// .drignore template that Initialize writes. Every manifest entry must
	// match the real file hash so that local == base and the plan is empty.
	manifestFiles := make(map[string]wapi.FileMeta)

	for rel := range files {
		hash, size, err := hashLocal(t, dir, rel)
		require.NoError(t, err)

		manifestFiles[rel] = wapi.FileMeta{Hash: hash, Size: size}
	}

	// Include .drignore (created by Initialize) so it too is in sync.
	hash, size, err := hashLocal(t, dir, ignore.FileName)
	require.NoError(t, err)

	manifestFiles[ignore.FileName] = wapi.FileMeta{Hash: hash, Size: size}

	syncedAt := time.Now().UTC()
	manifest := wapi.Manifest{
		Version:         wapi.ManifestVersion,
		SyncedAt:        &syncedAt,
		SyncedVersionID: &versionID,
		Files:           manifestFiles,
	}

	require.NoError(t, wapi.SaveManifest(dir, manifest))

	return dir
}

// modifyFile rewrites a file in dir with new content, for tests that need
// pending changes after a syncedProject setup.
func modifyFile(t *testing.T, dir, rel, content string) {
	t.Helper()

	abs := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}
