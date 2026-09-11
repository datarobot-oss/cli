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

package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/require"
)

const (
	// testArtifactID matches the bare-hex shape of real DataRobot artifact ids.
	testArtifactID = "6a90da2ddeadbeefcafe1234"

	// testCatalogID and testVersionID are syntactically valid stand-ins for
	// the catalog and catalog-version pointers recorded after a real sync.
	testCatalogID = "65f1a2b3c4d5e6f7a8b9c0d1"
	testVersionID = "65f1a2b3c4d5e6f7a8b9c0d2"
)

// testHash returns a 64-char lowercase hex string for manifest FileMeta
// fixtures (mirrors the wapi package's own test helper, which cannot be
// imported across package boundaries).
func testHash(c byte) string {
	return strings.Repeat(string(c), 64)
}

// strPtr returns a pointer to s, a test helper for optional config fields.
func strPtr(s string) *string {
	return &s
}

// initStateDir creates an empty state directory at the current location so
// wapi presence succeeds without any state files inside it.
func initStateDir(t *testing.T, projectDir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(wapi.Dir(projectDir), 0o755))
}

// validConfig builds a config that passes wapi semantic validation. Empty
// catalogID/lastSyncedVersionID leave the corresponding pointer nil.
func validConfig(catalogID, lastSyncedVersionID string) wapi.Config {
	cfg := wapi.Config{
		ArtifactID: testArtifactID,
		CreatedAt:  time.Now().UTC(),
		CLIVersion: "test-version",
	}

	if catalogID != "" {
		cfg.CatalogID = strPtr(catalogID)
	}

	if lastSyncedVersionID != "" {
		cfg.LastSyncedVersionID = strPtr(lastSyncedVersionID)
	}

	return cfg
}

// validConfigJSON renders a hand-written config.json body that passes wapi
// validation, for tests that fabricate raw file contents.
func validConfigJSON() string {
	return `{"artifactId":"` + testArtifactID + `","createdAt":"2026-01-01T00:00:00Z","cliVersion":"test-version"}`
}

// validManifest builds a manifest that passes wapi semantic validation. When
// syncedVersionID is empty the synced pointers stay nil (both-or-neither).
func validManifest(syncedVersionID string) wapi.Manifest {
	m := wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{"app/main.go": {Hash: testHash('a'), Size: 3}},
	}

	if syncedVersionID != "" {
		now := time.Now().UTC()

		m.SyncedAt = &now

		m.SyncedVersionID = strPtr(syncedVersionID)
	}

	return m
}

// writeStateFile writes raw contents to a file inside the state directory
// (creating the directory first). wapi.Dir resolves to the legacy location
// when only a legacy directory exists, which legacy-path fixtures rely on.
func writeStateFile(t *testing.T, projectDir, name, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(wapi.Dir(projectDir), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(wapi.Dir(projectDir), name), []byte(contents), 0o600))
}

// stateFileHashes maps every file under projectDir to its SHA-256 hex digest.
// The read-only guarantee tests compare snapshots taken before and after a
// check run: identical maps prove zero writes and zero new files. Reads go
// through an os.Root scoped to projectDir so the walk cannot be raced into
// following a symlink outside the project.
func stateFileHashes(t *testing.T, projectDir string) map[string]string {
	t.Helper()

	root, err := os.OpenRoot(projectDir)

	require.NoError(t, err)

	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Logf("close project root: %v", closeErr)
		}
	}()

	hashes := map[string]string{}

	err = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(projectDir, path)
		if err != nil {
			return err
		}

		f, err := root.Open(rel)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(f)

		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}

		if err != nil {
			return err
		}

		sum := sha256.Sum256(data)

		hashes[path] = hex.EncodeToString(sum[:])

		return nil
	})

	require.NoError(t, err)

	return hashes
}

// runLocalChecks runs the full local check suite through the framework
// Runner and returns the results in the fixed check order.
func runLocalChecks(t *testing.T, projectDir string) []core.Result {
	t.Helper()

	return core.NewRunner(LocalChecks(projectDir)...).Run(context.Background())
}
