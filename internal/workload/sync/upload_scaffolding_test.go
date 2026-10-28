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

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertZeroUploadCalls asserts that no upload-side network calls were issued
// on the fake: stage create, upload-to-stage, apply-stage, and zip upload all
// zero. This is the core assertion for VAL-UPLOAD-015 and VAL-UPLOAD-017.
func assertZeroUploadCalls(t *testing.T, fake *fakeFilesClient) {
	t.Helper()

	assert.Equal(t, 0, fake.CreateStageCalls(), "CreateStage must not be called")
	assert.Equal(t, 0, fake.UploadToStageCalls(), "UploadToStage must not be called")
	assert.Equal(t, 0, fake.ApplyStageCalls(), "ApplyStage must not be called")
	assert.Equal(t, 0, fake.UploadFromZipCalls(), "UploadFromZip must not be called")
}

// TestEmptyPlan_ZeroUploadCalls verifies VAL-UPLOAD-015: when the plan has
// zero uploads (all files unchanged), no upload API calls are issued. The
// per-method counters on the fake prove this at the Go level.
func TestEmptyPlan_ZeroUploadCalls(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":  "print('hi')\n",
		"main.py": "def main(): pass\n",
	}, catalogID, versionID)

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-should-not-be-used",
		versionID: "ver-should-not-be-used",
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
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

	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	// An empty plan (all files unchanged) issues zero upload calls.
	assertZeroUploadCalls(t, fake)
}

// TestDryRun_ZeroUploadCalls verifies VAL-UPLOAD-017: a --dry-run run with
// pending changes issues zero upload-side calls on the fake. The plan is
// non-empty (a file was modified), but Run returns after Plan without calling
// Execute, so no upload methods are invoked.
func TestDryRun_ZeroUploadCalls(t *testing.T) {
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

	// Run stops after Plan because DryRun is true. The plan is non-empty
	// (a file was modified), but Execute is never called, so no upload
	// methods are invoked. The positive control (TestNonEmptyPlan_UploadCallsAreNonZero)
	// proves the same setup with DryRun=false DOES issue upload calls, so
	// the zero-count here is not vacuously true.
	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	// No upload-side calls despite a non-empty plan.
	assertZeroUploadCalls(t, fake)
}

// TestNonEmptyPlan_UploadCallsAreNonZero is the positive control for
// VAL-UPLOAD-015 and VAL-UPLOAD-017: a non-empty plan in a non-dry-run sync
// DOES issue upload-side calls. This proves the counters work and the
// zero-call assertions above are not vacuously true.
func TestNonEmptyPlan_UploadCallsAreNonZero(t *testing.T) {
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

	// A non-empty plan in a non-dry-run sync must issue upload calls.
	assert.Equal(t, 1, fake.CreateStageCalls(), "CreateStage must be called for a non-empty plan")
	assert.Positive(t, fake.UploadToStageCalls(), "UploadToStage must be called for a non-empty plan")
	assert.Equal(t, 1, fake.ApplyStageCalls(), "ApplyStage must be called for a non-empty plan")
	assert.Equal(t, 0, fake.UploadFromZipCalls(), "zip path must not be used for a small upload")
}
