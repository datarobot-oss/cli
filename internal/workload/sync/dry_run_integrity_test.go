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

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRun_LeavesStateUntouched verifies that a --dry-run run with pending
// changes leaves manifest.json (content AND mtime), config.json, and the
// fake's server state all unchanged. The plan is non-empty (a file was
// modified), but Run returns after Plan without calling Execute, so no
// upload-side calls are issued and no state files are written.
//
// Go-level coverage:
//   - manifest.json content is byte-identical before and after.
//   - manifest.json mtime is unchanged (SaveManifest is never called; the
//     file is not touched at all, so even the mtime survives).
//   - config.json content is byte-identical (SaveConfig is never called).
//   - The fake's upload counters are all zero (no stage, upload, apply, or
//     zip calls).
//   - AllFiles is not called (the artifact is not drifted, so the fast path
//     copies BASE to REMOTE without a round-trip).
//
// What remains for a staging validator to confirm through the real binary:
//   - Real mtime preservation across a process invocation (a Go test shares
//     a process and a filesystem cache; a separate `dr` process touching
//     the file would reset the mtime even if the content is unchanged).
//   - The server's AllFiles listing is unchanged after the dry-run (the Go
//     test asserts zero upload calls, which implies this, but the staging
//     validator confirms it against the real server).
//
// Fulfills VAL-UPLOAD-016.
func TestDryRun_LeavesStateUntouched(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	// Introduce a pending change so the plan is non-empty.
	modifyFile(t, dir, "app.py", "print('changed')\n")

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-should-not-be-used",
		versionID: "ver-should-not-be-used",
	}

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	// Capture pre-run state: manifest bytes, manifest mtime, config bytes.
	mPath := filepath.Join(wapi.Dir(dir), "manifest.json")

	preManifest, err := os.ReadFile(mPath)
	require.NoError(t, err)

	// Set a known mtime well in the past so any touch (even at the same
	// content) is detectable regardless of filesystem mtime resolution.
	knownTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(mPath, knownTime, knownTime))

	preInfo, err := os.Stat(mPath)
	require.NoError(t, err)

	preManifestMtime := preInfo.ModTime()

	preConfig, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	// Run the dry-run. It must stop after Plan (DryRun is true).
	result, err := e.Run()
	require.NoError(t, err)

	require.NotNil(t, result, "dry-run must return a result")

	// The plan must be non-empty — the positive control that proves the
	// zero-call assertions below are not vacuously true. Read the plan Run
	// already computed (Plan stores it on the engine) rather than re-running
	// Plan() after the fact: a second Plan is a full re-execution of phases
	// 0-4 and could itself issue calls or touch state, which would silently
	// couple the zero-call assertions to plan-phase purity. Reading the
	// stored plan is passive and keeps the control independent of that
	// assumption.
	require.NotNil(t, e.plan, "Run must have computed a plan for the control to mean anything")

	assert.False(t, e.plan.IsEmpty(),
		"plan must be non-empty (a file was modified) for the zero-call assertions to be meaningful")

	// manifest.json content must be byte-identical.
	postManifest, err := os.ReadFile(mPath)
	require.NoError(t, err)

	assert.Equal(t, preManifest, postManifest,
		"manifest.json content must be unchanged after a dry-run")

	// manifest.json mtime must be unchanged — SaveManifest was never called.
	postInfo, err := os.Stat(mPath)
	require.NoError(t, err)

	assert.True(t, postInfo.ModTime().Equal(preManifestMtime),
		"manifest.json mtime must be unchanged after a dry-run (SaveManifest not called)")

	// config.json content must be byte-identical.
	postConfig, err := os.ReadFile(wapi.ConfigPath(dir))
	require.NoError(t, err)

	assert.Equal(t, preConfig, postConfig,
		"config.json content must be unchanged after a dry-run")

	// No upload-side calls: the fake's server state is untouched.
	assert.Equal(t, 0, fake.CreateStageCalls(),
		"CreateStage must not be called in a dry-run")
	assert.Equal(t, 0, fake.UploadToStageCalls(),
		"UploadToStage must not be called in a dry-run")
	assert.Equal(t, 0, fake.ApplyStageCalls(),
		"ApplyStage must not be called in a dry-run")
	assert.Equal(t, 0, fake.UploadFromZipCalls(),
		"UploadFromZip must not be called in a dry-run")
	assert.Equal(t, 0, fake.AllFilesCalls(),
		"AllFiles must not be called in a dry-run on a non-drifted artifact (fast path)")
}
