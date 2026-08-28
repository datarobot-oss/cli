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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newArtifactID is a second bare-hex id distinct from testArtifactID, used as
// the relink target in happy-path tests.
const newArtifactID = "6a90da2ddeadbeefcafe5678"

// newCatalogID is a second catalog id distinct from testCatalogID.
const newCatalogID = "65f1a2b3c4d5e6f7a8b9c0d3"

// fixedTime is a deterministic clock for history-entry timestamp assertions.
var fixedTime = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// alwaysConfirm is a RelinkConfirmFunc that always proceeds.
func alwaysConfirm(_ string) bool { return true }

// neverConfirm is a RelinkConfirmFunc that always declines.
func neverConfirm(_ string) bool { return false }

// fakeStore returns an ArtifactGetter that always returns the given artifact.
func fakeStore(art *workload.Artifact) ArtifactGetter {
	return ArtifactGetterFunc(func(string) (*workload.Artifact, error) {
		return art, nil
	})
}

// errorStore returns an ArtifactGetter that always returns the given error.
func errorStore(err error) ArtifactGetter {
	return ArtifactGetterFunc(func(string) (*workload.Artifact, error) {
		return nil, err
	})
}

// makeArtifact builds an artifact fixture with the given id, status, and
// optional codeRef planted on the primary container.
func makeArtifact(id, status string, codeRef *workload.DatarobotCodeRef) *workload.Artifact {
	art := &workload.Artifact{
		ID:     id,
		Name:   "doctor-test",
		Status: status,
	}

	if codeRef == nil {
		return art
	}

	primary := true

	art.Spec.ContainerGroups = []workload.ContainerGroup{
		{
			Containers: []workload.Container{
				{
					Primary: &primary,
					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{Datarobot: codeRef},
					},
				},
			},
		},
	}

	return art
}

// fakeDraftArtifact returns a draft service artifact with an optional codeRef.
func fakeDraftArtifact(id string, codeRef *workload.DatarobotCodeRef) *workload.Artifact {
	return makeArtifact(id, "DRAFT", codeRef)
}

// relinkOpts builds a RelinkOptions with sensible defaults for tests.
func relinkOpts(dir, newID string, store ArtifactGetter, confirm RelinkConfirmFunc) RelinkOptions {
	return RelinkOptions{
		ProjectDir:    dir,
		NewArtifactID: newID,
		Store:         store,
		Confirm:       confirm,
		Goos:          runtime.GOOS,
		Now:           func() time.Time { return fixedTime },
	}
}

// linkedProject creates a temp dir with a valid linked state (config + manifest
// + history) pointing at testArtifactID. The catalogId and lastSyncedVersionId
// are populated to simulate a post-sync state.
func linkedProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))
	require.NoError(t, wapi.SaveManifest(dir, validManifest(testVersionID)))

	// Seed a history.log with an init entry.
	require.NoError(t, wapi.AppendHistory(dir, wapi.HistoryEntry{
		"op": "init", "ts": "2026-01-01T00:00:00Z",
	}))

	return dir
}

// linkedDraftProject creates a temp dir with a valid never-synced linked state
// (no catalog pointers, empty manifest).
func linkedDraftProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))
	require.NoError(t, wapi.SaveManifest(dir, wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{},
	}))

	return dir
}

// TestRunRelink_HappyPath_RepointsWithFreshBase covers VAL-RELINK-001:
// relink from an old artifact to a new live draft artifact repoints config,
// resets manifest to empty BASE, and appends a relink history entry.
func TestRunRelink_HappyPath_RepointsWithFreshBase(t *testing.T) {
	dir := linkedProject(t)

	// Place a working-tree file that must survive untouched.
	working := filepath.Join(dir, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("user source"), 0o600))

	before := stateFileHashes(t, dir)

	target := fakeDraftArtifact(newArtifactID, &workload.DatarobotCodeRef{
		CatalogID:        newCatalogID,
		CatalogVersionID: "65f1a2b3c4d5e6f7a8b9c0d4",
	})

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	require.Len(t, actions, 1)

	assert.Equal(t, RelinkActionID, actions[0].ID)
	assert.Equal(t, core.ActionPerformed, actions[0].Status)
	assert.Contains(t, actions[0].Reason, testArtifactID)
	assert.Contains(t, actions[0].Reason, newArtifactID)

	// Config: artifactId=new, catalogId=new codeRef.CatalogID, lsv=nil.
	cfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Equal(t, newArtifactID, cfg.ArtifactID)

	require.NotNil(t, cfg.CatalogID)

	assert.Equal(t, newCatalogID, *cfg.CatalogID)

	assert.Nil(t, cfg.LastSyncedVersionID, "lastSyncedVersionId must be nil (fresh BASE)")

	// Manifest: empty BASE (files={}, synced fields nil).
	m, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	assert.Empty(t, m.Files)
	assert.Nil(t, m.SyncedVersionID)
	assert.Nil(t, m.SyncedAt)
	assert.Equal(t, wapi.ManifestVersion, m.Version)

	// History: last line is {op:relink, from, to, ts}.
	history, err := os.ReadFile(filepath.Join(wapi.Dir(dir), "history.log"))

	require.NoError(t, err)

	lines := splitLines(string(history))

	last := lines[len(lines)-1]

	assert.Contains(t, last, `"op":"relink"`)
	assert.Contains(t, last, `"from":"`+testArtifactID+`"`)
	assert.Contains(t, last, `"to":"`+newArtifactID+`"`)
	assert.Contains(t, last, `"ts":"`+fixedTime.Format(time.RFC3339)+`"`)

	// Working tree untouched: the project file is byte-identical.
	after := stateFileHashes(t, dir)

	workingAfter, ok := after[working]

	require.True(t, ok)

	assert.Equal(t, before[working], workingAfter, "working-tree file must be untouched")
}

// TestRunRelink_FreshInit_CatalogIdFromTargetOrNil covers VAL-RELINK-011:
// relink from a fresh init (no prior sync) to a new artifact with no codeRef
// leaves catalogId nil and lsv nil.
func TestRunRelink_FreshInit_CatalogIdFromTargetOrNil(t *testing.T) {
	dir := linkedDraftProject(t)

	target := fakeDraftArtifact(newArtifactID, nil) // no codeRef

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	assert.Equal(t, core.ActionPerformed, actions[0].Status)

	cfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Equal(t, newArtifactID, cfg.ArtifactID)
	assert.Nil(t, cfg.CatalogID, "no codeRef means catalogId is nil")
	assert.Nil(t, cfg.LastSyncedVersionID)
}

// TestRunRelink_EmptyCodeRef_NormalizedToNil covers the empty-vs-nil
// normalization: a codeRef with empty CatalogID field normalizes to nil.
func TestRunRelink_EmptyCodeRef_NormalizedToNil(t *testing.T) {
	dir := linkedDraftProject(t)

	// codeRef with empty CatalogID (not nil, but empty string).
	target := fakeDraftArtifact(newArtifactID, &workload.DatarobotCodeRef{
		CatalogID:        "",
		CatalogVersionID: "",
	})

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	assert.Equal(t, core.ActionPerformed, actions[0].Status)

	cfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Nil(t, cfg.CatalogID, "empty codeRef.CatalogID must normalize to nil")
}

// TestRunRelink_PopulatedBaseWiped covers VAL-RELINK-012 and VAL-RELINK-018:
// a populated manifest (files + synced pointers) is wiped to empty BASE.
func TestRunRelink_PopulatedBaseWiped(t *testing.T) {
	dir := linkedProject(t)

	// Manifest has files and synced pointers.
	mBefore, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	assert.NotEmpty(t, mBefore.Files)
	require.NotNil(t, mBefore.SyncedVersionID)

	target := fakeDraftArtifact(newArtifactID, &workload.DatarobotCodeRef{
		CatalogID: newCatalogID,
	})

	_, err = RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	mAfter, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	assert.Empty(t, mAfter.Files, "files must be wiped")
	assert.Nil(t, mAfter.SyncedVersionID, "syncedVersionId must be nil")
	assert.Nil(t, mAfter.SyncedAt, "syncedAt must be nil")
}

// TestRunRelink_SameID_AllowedWarnedBaseReset covers VAL-RELINK-009:
// relinking to the same artifact id is allowed, warned, and resets BASE.
func TestRunRelink_SameID_AllowedWarnedBaseReset(t *testing.T) {
	dir := linkedProject(t)

	warningText := ""

	captureConfirm := func(warning string) bool {
		warningText = warning

		return true
	}

	target := fakeDraftArtifact(testArtifactID, &workload.DatarobotCodeRef{
		CatalogID: newCatalogID, // different catalog to verify refresh
	})

	actions, err := RunRelink(context.Background(), RelinkOptions{
		ProjectDir:    dir,
		NewArtifactID: testArtifactID, // same id
		Store:         fakeStore(target),
		Confirm:       captureConfirm,
		Goos:          runtime.GOOS,
		Now:           func() time.Time { return fixedTime },
	})

	require.NoError(t, err)

	assert.Equal(t, core.ActionPerformed, actions[0].Status)

	// Warning includes the same-id note.
	assert.Contains(t, warningText, "same artifact")
	assert.Contains(t, warningText, testArtifactID)

	// Config: artifactId unchanged, catalogId refreshed, lsv nil.
	cfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Equal(t, testArtifactID, cfg.ArtifactID, "artifactId unchanged")

	require.NotNil(t, cfg.CatalogID)

	assert.Equal(t, newCatalogID, *cfg.CatalogID, "catalogId refreshed from target codeRef")

	assert.Nil(t, cfg.LastSyncedVersionID, "lsv reset to nil")

	// Manifest: empty BASE.
	m, err := wapi.LoadManifest(dir)

	require.NoError(t, err)

	assert.Empty(t, m.Files)

	// History: from == to.
	history, _ := os.ReadFile(filepath.Join(wapi.Dir(dir), "history.log"))

	assert.Contains(t, string(history), `"from":"`+testArtifactID+`"`)
	assert.Contains(t, string(history), `"to":"`+testArtifactID+`"`)
}

// TestRunRelink_NotLinked_ErrorPointsToInit covers VAL-RELINK-010:
// relink on a not-linked project returns ErrRelinkNotLinked without fetching.
func TestRunRelink_NotLinked_ErrorPointsToInit(t *testing.T) {
	dir := t.TempDir()

	calls := 0

	store := ArtifactGetterFunc(func(string) (*workload.Artifact, error) {
		calls++

		return fakeDraftArtifact(newArtifactID, nil), nil
	})

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, store, alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkNotLinked)

	assert.Nil(t, actions)

	assert.Zero(t, calls, "no network fetch for a not-linked project")

	// No state dir created.
	_, statErr := os.Stat(wapi.Dir(dir))

	assert.ErrorIs(t, statErr, os.ErrNotExist, "no state dir created")
}

// TestRunRelink_404Target_AbortsStateUntouched covers VAL-RELINK-006:
// a 404 target aborts with a skipped action and state untouched.
func TestRunRelink_404Target_AbortsStateUntouched(t *testing.T) {
	dir := linkedProject(t)

	before := stateFileHashes(t, dir)

	store := errorStore(&drapi.HTTPError{StatusCode: 404, URL: "https://test/artifacts/x/"})

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, store, alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkAbort)

	require.Len(t, actions, 1)

	assert.Equal(t, core.ActionSkipped, actions[0].Status)
	assert.Contains(t, actions[0].Reason, "not found")

	// State byte-identical.
	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_LockedTarget_AbortsStateUntouched covers VAL-RELINK-007:
// a locked target aborts with a skipped action and writes nothing.
func TestRunRelink_LockedTarget_AbortsStateUntouched(t *testing.T) {
	dir := linkedProject(t)

	before := stateFileHashes(t, dir)

	target := makeArtifact(newArtifactID, "LOCKED", nil)

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkAbort)

	require.Len(t, actions, 1)

	assert.Equal(t, core.ActionSkipped, actions[0].Status)
	assert.Contains(t, actions[0].Reason, "locked")

	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_WrongType_AbortsStateUntouched covers VAL-RELINK-023:
// a non-service artifact type aborts with a skipped action.
func TestRunRelink_WrongType_AbortsStateUntouched(t *testing.T) {
	dir := linkedProject(t)

	before := stateFileHashes(t, dir)

	target := &workload.Artifact{
		ID:     newArtifactID,
		Name:   "agent-fixture",
		Status: "DRAFT",
		Type:   "agent",
	}

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkAbort)

	require.Len(t, actions, 1)

	assert.Equal(t, core.ActionSkipped, actions[0].Status)
	assert.Contains(t, actions[0].Reason, "agent")
	assert.Contains(t, actions[0].Reason, "service")

	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_EmptyTypeDefaultsToService covers the ArtifactTypeOrDefault
// behavior: an empty type defaults to service and passes the type gate.
func TestRunRelink_EmptyTypeDefaultsToService(t *testing.T) {
	dir := linkedDraftProject(t)

	target := &workload.Artifact{
		ID:     newArtifactID,
		Name:   "no-type-fixture",
		Status: "DRAFT",
		Type:   "", // empty defaults to service
	}

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	assert.Equal(t, core.ActionPerformed, actions[0].Status)
}

// TestRunRelink_APIUnreachable_AbortsStateUntouched covers the API unreachable
// gate: any non-404 fetch error aborts with ErrRelinkAPIUnreachable.
func TestRunRelink_APIUnreachable_AbortsStateUntouched(t *testing.T) {
	dir := linkedProject(t)

	before := stateFileHashes(t, dir)

	store := errorStore(errors.New("dial tcp 127.0.0.1:443: connect: connection refused"))

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, store, alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkAPIUnreachable)

	assert.Nil(t, actions, "no actions for an API unreachable abort")

	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_LockHeld_AbortsStateUntouched covers VAL-RELINK-021:
// a held sync lock aborts the relink with "sync in progress" and state untouched.
func TestRunRelink_LockHeld_AbortsStateUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows gate path is covered by the seam tests")
	}

	dir := linkedProject(t)

	require.NoError(t, os.WriteFile(lockPath(t, dir), nil, 0o600))

	before := stateFileHashes(t, dir)

	release := holdLockForTest(t, dir)

	defer release()

	target := fakeDraftArtifact(newArtifactID, nil)

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.ErrorIs(t, err, ErrRelinkAbort)

	require.Len(t, actions, 1)

	assert.Equal(t, core.ActionSkipped, actions[0].Status)
	assert.Contains(t, actions[0].Reason, "sync in progress")

	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_Declined_AbortsStateUntouched covers VAL-RELINK-003:
// declining the confirm prompt aborts with state untouched.
func TestRunRelink_Declined_AbortsStateUntouched(t *testing.T) {
	dir := linkedProject(t)

	before := stateFileHashes(t, dir)

	target := fakeDraftArtifact(newArtifactID, nil)

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), neverConfirm))

	require.ErrorIs(t, err, ErrRelinkAbort)

	require.Len(t, actions, 1)

	assert.Equal(t, core.ActionSkipped, actions[0].Status)
	assert.Contains(t, actions[0].Reason, "declined")

	assert.Equal(t, before, stateFileHashes(t, dir))
}

// TestRunRelink_WindowsGate_Proceeds pins the windows gate behavior: the lock
// probe SKIPs (flock not enforced), and the relink still proceeds.
func TestRunRelink_WindowsGate_Proceeds(t *testing.T) {
	dir := linkedDraftProject(t)

	target := fakeDraftArtifact(newArtifactID, nil)

	opts := relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm)
	opts.Goos = "windows"

	actions, err := RunRelink(context.Background(), opts)

	require.NoError(t, err)

	assert.Equal(t, core.ActionPerformed, actions[0].Status)
}

// TestRunRelink_RepeatedRelink_LastWins covers VAL-RELINK-024:
// relink A→B then B→C leaves config pointing at C with two history entries.
func TestRunRelink_RepeatedRelink_LastWins(t *testing.T) {
	dir := linkedDraftProject(t)

	thirdID := "6a90da2ddeadbeefcafe9999"

	// A→B
	targetB := fakeDraftArtifact(newArtifactID, nil)

	_, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(targetB), alwaysConfirm))

	require.NoError(t, err)

	// B→C
	targetC := fakeDraftArtifact(thirdID, nil)

	opts := relinkOpts(dir, thirdID, fakeStore(targetC), alwaysConfirm)

	_, err = RunRelink(context.Background(), opts)

	require.NoError(t, err)

	cfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Equal(t, thirdID, cfg.ArtifactID)

	// Two relink history entries.
	history, _ := os.ReadFile(filepath.Join(wapi.Dir(dir), "history.log"))

	lines := splitLines(string(history))

	var relinkLines []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		if line == "" {
			continue
		}

		if contains(line, `"op":"relink"`) {
			relinkLines = append(relinkLines, line)
		}
	}

	require.Len(t, relinkLines, 2, "exactly two relink entries")

	assert.Contains(t, relinkLines[0], `"to":"`+newArtifactID+`"`)
	assert.Contains(t, relinkLines[1], `"to":"`+thirdID+`"`)
}

// TestRunRelink_PreservesCreatedAtAndCLIVersion pins that the relink does not
// reset the config's createdAt or cliVersion fields.
func TestRunRelink_PreservesCreatedAtAndCLIVersion(t *testing.T) {
	dir := linkedProject(t)

	oldCfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	target := fakeDraftArtifact(newArtifactID, nil)

	_, err = RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.NoError(t, err)

	newCfg, err := wapi.LoadConfig(dir)

	require.NoError(t, err)

	assert.Equal(t, oldCfg.CreatedAt, newCfg.CreatedAt, "createdAt must be preserved")
	assert.Equal(t, oldCfg.CLIVersion, newCfg.CLIVersion, "cliVersion must be preserved")
}

// TestRunRelink_CorruptConfig_Aborts covers the case where the config is
// corrupt: the relink cannot read the old artifact id and aborts with an error.
func TestRunRelink_CorruptConfig_Aborts(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	writeStateFile(t, dir, "config.json", `{"artifactId":"abc`)

	target := fakeDraftArtifact(newArtifactID, nil)

	actions, err := RunRelink(context.Background(), relinkOpts(dir, newArtifactID, fakeStore(target), alwaysConfirm))

	require.Error(t, err)

	assert.Nil(t, actions)

	assert.Contains(t, err.Error(), "doctor --fix")
}

// splitLines splits a string on newlines, dropping trailing empty lines.
func splitLines(s string) []string {
	lines := []string{}

	for _, line := range splitNewlines(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// splitNewlines splits on \n without allocating a trailing empty element.
func splitNewlines(s string) []string {
	var lines []string

	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])

			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

// contains is a simple substring check (avoids importing strings in test).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
