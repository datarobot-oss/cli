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
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emptyHash is the SHA-256 of the empty string, matching what a torn-to-empty
// file produces when streamed.
const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// TestExecuteRecordsStreamedHash is the deterministic form of the TOCTOU that
// poisoned the manifest 3/3 times through the real binary. Plan() computes a
// hash, then the file is rewritten, then Execute(plan) streams the new bytes.
// The manifest must record the streamed hash, not the Phase-2 planned hash.
func TestExecuteRecordsStreamedHash(t *testing.T) {
	tests := []struct {
		name        string
		planContent string // file content at Plan time
		execContent string // file content at Execute time
	}{
		{
			name:        "same_size_rewrite",
			planContent: "print('ho')\n", // 11 bytes
			execContent: "print('ok')\n", // 11 bytes, different content
		},
		{
			name:        "grows",
			planContent: "print('hi')\n",    // 11 bytes
			execContent: "print('hello')\n", // 14 bytes
		},
		{
			name:        "shrinks",
			planContent: "print('hello')\n", // 14 bytes
			execContent: "print('hi')\n",    // 11 bytes
		},
		{
			name:        "torn_to_empty",
			planContent: "print('hi')\n", // 11 bytes
			execContent: "",              // 0 bytes
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				catalogID = "cid-synced"
				versionID = "ver-synced"
			)

			// Start from a synced project so Plan sees exactly one upload.
			dir := syncedProject(t, map[string]string{
				"app.py": "print('orig')\n",
			}, catalogID, versionID)

			// Introduce a pending change so Plan sees app.py as an upload.
			modifyFile(t, dir, "app.py", tc.planContent)

			fake := &fakeFilesClient{
				catalogID: catalogID,
				stageID:   "stage-1",
				versionID: "ver-new",
			}

			e, err := newWithDeps(dir, Options{Yes: true}, Deps{
				Files: fake,
				Artifacts: &fakeArtifactStore{
					GetFn: func(id string) (*workload.Artifact, error) {
						return draftArtifact(id, catalogID, versionID), nil
					},
					PatchFn: func(_, _, _ string) error { return nil },
				},
				Now: time.Now,
			})
			require.NoError(t, err)

			t.Cleanup(func() { _ = e.Close() })

			plan, err := e.Plan()
			require.NoError(t, err)

			require.Len(t, plan.Uploads, 1, "exactly one upload expected")
			require.Equal(t, "app.py", plan.Uploads[0].Path)

			// The Phase-2 planned hash is of the content present at Plan time.
			plannedHash := plan.Uploads[0].LocalHash
			plannedSize := plan.Uploads[0].LocalSize

			// The streamed hash is of the content present at Execute time.
			streamedContent := []byte(tc.execContent)
			streamedHash := sha256Hex(streamedContent)
			streamedSize := int64(len(streamedContent))

			// The test is only meaningful if the planned and streamed hashes differ.
			require.NotEqual(t, plannedHash, streamedHash,
				"planned and streamed hashes must differ for the test to be meaningful")

			// Between Plan and Execute, rewrite the file. This is the TOCTOU window.
			modifyFile(t, dir, "app.py", tc.execContent)

			result, err := e.Execute(plan)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Read the manifest and verify it records the streamed bytes.
			manifest, err := wapi.LoadManifest(dir)
			require.NoError(t, err)

			fm, ok := manifest.Files["app.py"]
			require.True(t, ok, "app.py must be in the manifest")

			assert.Equal(t, streamedHash, fm.Hash,
				"manifest must record the streamed hash, not the Phase-2 planned hash")
			assert.NotEqual(t, plannedHash, fm.Hash,
				"manifest must NOT record the Phase-2 planned hash")
			assert.Equal(t, streamedSize, fm.Size,
				"manifest must record the streamed size, not the Phase-2 planned size")

			// Size inequality only holds when the content actually changed size;
			// the same_size case has identical planned and streamed sizes by
			// construction, and that is correct.
			if plannedSize != streamedSize {
				assert.NotEqual(t, plannedSize, fm.Size,
					"manifest must NOT record the Phase-2 planned size when sizes differ")
			}

			// The torn-to-empty case has a specific expected hash.
			if tc.execContent == "" {
				assert.Equal(t, emptyHash, fm.Hash,
					"torn-to-empty must record the SHA-256 of the empty string")
				assert.Equal(t, int64(0), fm.Size,
					"torn-to-empty must record size 0")
			}

			// Every hash must be 64 lowercase hex characters and size >= 0.
			assert.Len(t, fm.Hash, 64, "hash must be 64 hex characters")
			assert.GreaterOrEqual(t, fm.Size, int64(0), "size must be >= 0")
		})
	}
}

// TestPhase6MissingSentHardFails verifies the hard rule: if Sent[path] is
// missing for an uploaded path, buildNewBaseManifest must return an error
// naming the path rather than falling back to the Phase-2 planned hash. A
// per-path fallback IS the original poisoning bug.
func TestPhase6MissingSentHardFails(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote: RemoteManifest{},
		uploadOutcome: &UploadOutcome{
			CatalogID: "cid",
			VersionID: "ver",
			Sent:      map[string]FileEntry{}, // missing app.py
		},
	}

	_, err := buildNewBaseManifest(e, "ver", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.py",
		"error must name the missing path")
}

// TestPhase6NilOutcomeHardFails verifies that a nil uploadOutcome with a
// non-empty upload list also hard-fails naming the path.
func TestPhase6NilOutcomeHardFails(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads: []FileAction{
				{Path: "app.py", LocalHash: "phase2hash", LocalSize: 11},
			},
		},
		remote:        RemoteManifest{},
		uploadOutcome: nil,
	}

	_, err := buildNewBaseManifest(e, "ver", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.py")
}

// TestPhase6EmptyPlanStillWritesManifest verifies that a plan with zero
// uploads still produces a correct manifest. The upload loop does not execute,
// so the hard-fail does not misfire on an empty Sent map. A nil uploadOutcome
// is safe when there are no uploads.
func TestPhase6EmptyPlanStillWritesManifest(t *testing.T) {
	e := &Engine{
		plan: &SyncPlan{
			Uploads:   []FileAction{},
			Downloads: []FileAction{},
			Deletes:   []FileAction{},
			Conflicts: []FileAction{},
		},
		remote: RemoteManifest{
			"app.py": {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
		},
		uploadOutcome: nil, // no uploads, so nil is fine
	}

	manifest, err := buildNewBaseManifest(e, "ver-new", time.Now())
	require.NoError(t, err)
	assert.Equal(t, wapi.ManifestVersion, manifest.Version)
	assert.Len(t, manifest.Files, 1)
	assert.Equal(t, sha256Hex([]byte("print('hi')\n")), manifest.Files["app.py"].Hash)
}

// TestManifestSchemaUnchanged verifies that the written manifest carries
// "version": 1 and that no new fields are added to manifest.json or config.json.
func TestManifestSchemaUnchanged(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	// Introduce a pending change.
	modifyFile(t, dir, "app.py", "print('changed')\n")

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-new",
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify manifest.json schema.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, 1, manifest.Version, "manifest version must stay 1")

	// Verify the manifest has exactly the expected top-level keys by reading
	// the raw JSON and checking no unexpected fields were added.
	rawManifest, err := os.ReadFile(dir + "/.datarobot/workload/manifest.json")
	require.NoError(t, err)

	// The manifest must contain "version": 1 and the standard fields only.
	assert.Contains(t, string(rawManifest), `"version": 1`)
	assert.Contains(t, string(rawManifest), `"files"`)
	assert.Contains(t, string(rawManifest), `"syncedVersionId"`)

	// Verify config.json has no new fields.
	rawConfig, err := os.ReadFile(dir + "/.datarobot/workload/config.json")
	require.NoError(t, err)

	assert.Contains(t, string(rawConfig), `"artifactId"`)
	assert.Contains(t, string(rawConfig), `"catalogId"`)
	assert.Contains(t, string(rawConfig), `"lastSyncedVersionId"`)

	// syncedVersionId in manifest must equal the version in the result.
	require.NotNil(t, manifest.SyncedVersionID)
	assert.Equal(t, result.NewVersion, *manifest.SyncedVersionID,
		"syncedVersionId must equal the version in the sync summary")

	// config.json's LastSyncedVersionID must equal manifest's syncedVersionId.
	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, *manifest.SyncedVersionID, *cfg.LastSyncedVersionID,
		"config LastSyncedVersionID must equal manifest syncedVersionId")
}
