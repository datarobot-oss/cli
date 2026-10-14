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
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The state write works on a value copy of the config, which is only safe
// while wapi.Config has no reference-type field the phase edits in place.
// If this fails, a field like that was added: rebuild it inside the phase
// rather than writing through the shared pointer.
func TestPhase7ConfigCopyIsNotMutatedInPlace(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
		newVerID  = "ver-new"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	origCatalogPtr := cfg.CatalogID
	origVersionPtr := cfg.LastSyncedVersionID

	require.NotNil(t, origCatalogPtr)
	require.NotNil(t, origVersionPtr)

	e := &Engine{
		projectDir: dir,
		config:     cfg,
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{
			"app.py": {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
		},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid-new",
			VersionID: newVerID,
			Sent: map[string]FileEntry{
				"app.py": {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
			},
		},
		newCatalogID: "cid-new",
		newVersionID: newVerID,
		nowFn:        time.Now,
	}

	require.NoError(t, phase7State(e))

	require.NotNil(t, e.config.LastSyncedVersionID)
	assert.Equal(t, newVerID, *e.config.LastSyncedVersionID)
	require.NotNil(t, e.config.CatalogID)
	assert.Equal(t, "cid-new", *e.config.CatalogID)

	assert.Equal(t, versionID, *origVersionPtr, "the old LastSyncedVersionID target is untouched")
	assert.Equal(t, catalogID, *origCatalogPtr, "the old CatalogID target is untouched")
}
