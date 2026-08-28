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
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeValidConfig writes a hand-crafted config.json that passes wapi
// validation (a linked, never-synced draft) without touching the manifest.
func writeValidConfig(t *testing.T, projectDir string) {
	t.Helper()

	writeStateFile(t, projectDir, "config.json",
		`{"artifactId":"`+testArtifactID+`","createdAt":"2026-01-01T00:00:00Z","cliVersion":"test-version"}`)
}

// jsonAction mirrors one element of the pinned actions array:
// {id, status, reason}.
type jsonAction struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// jsonFixReport mirrors the doctor JSON report for repair runs: the base
// report plus the actions array (embedded so JSON keys flatten).
type jsonFixReport struct {
	jsonReport

	Actions []jsonAction `json:"actions"`
}

// TestRunE_FixHealthyProject_NothingToDo_ExitZero covers VAL-FIX-001: --fix
// on a healthy project is a no-op whose text output says "nothing to do"
// explicitly and exits 0.
func TestRunE_FixHealthyProject_NothingToDo_ExitZero(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--fix")

	outStr := mustRun(t, c, out)

	assert.Empty(t, errOut.String())
	assert.Contains(t, outStr, "Repairs")
	assert.Contains(t, outStr, "nothing to fix")
	assert.Contains(t, outStr, "verdict: ok")
}

// TestRunE_FixMissingManifest_PostFixOK_ExitZero covers VAL-FIX-002,
// VAL-FIX-013 and VAL-FIX-014 at the command surface: the repair is
// performed, the post-fix check suite reports the manifest OK, the exit code
// is 0, and the JSON stdout is pure with a pinned-shape actions array.
func TestRunE_FixMissingManifest_PostFixOK_ExitZero(t *testing.T) {
	tmp := t.TempDir()

	// Valid config, no manifest.json: the rebuild must repair it.
	writeValidConfig(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--fix", "--output-format", "json")

	outStr := mustRun(t, c, out)

	assert.Empty(t, errOut.String())

	var report jsonFixReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report), "stdout must be a single pure-JSON object")

	assert.Equal(t, "ok", report.Status, "post-fix state is healthy")
	assert.Equal(t, 10, report.Summary.OK)

	require.Len(t, report.Actions, 3)

	assert.Equal(t, "wapi.manifest", report.Actions[0].ID)
	assert.Equal(t, "performed", report.Actions[0].Status)
	assert.Equal(t, "wapi.rollback", report.Actions[1].ID)
	assert.Equal(t, "wapi.lock", report.Actions[2].ID)

	for _, check := range report.Checks {
		assert.Equal(t, "OK", check.Status, "post-fix check %s", check.ID)
	}

	// The rebuilt manifest parses and is an empty BASE (both-or-neither).
	m, err := wapi.LoadManifest(tmp)

	require.NoError(t, err)

	assert.Empty(t, m.Files)
	assert.Nil(t, m.SyncedVersionID)
	assert.Nil(t, m.SyncedAt)
}

// TestRunE_FixCorruptConfig_ManifestSkipped_ExitOne covers VAL-FIX-005 and
// VAL-FIX-015: a corrupt config makes the manifest rebuild skip with a
// re-init remedy, the unfixable FAIL keeps exit 1, and stdout stays pure
// JSON on the failure path.
func TestRunE_FixCorruptConfig_ManifestSkipped_ExitOne(t *testing.T) {
	tmp := t.TempDir()

	writeStateFile(t, tmp, "config.json", `{"artifactId":"abc`)

	c, out, _ := newTestCmd(t, "--dir", tmp, "--fix", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "an unfixable FAIL keeps exit 1")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON on failure")

	assert.Equal(t, "fail", report.Status)

	require.NotEmpty(t, report.Actions)

	rebuild := report.Actions[0]

	assert.Equal(t, "wapi.manifest", rebuild.ID)
	assert.Equal(t, "skipped", rebuild.Status)
	assert.Contains(t, rebuild.Reason, "config")
	assert.Contains(t, rebuild.Reason, "init", "the skip reason must carry the re-init remedy")
}

// TestRunE_FixHeldLock_AllSkipped_ExitOne covers VAL-FIX-008, VAL-FIX-018
// and VAL-CROSS-014 at the command surface: a live holder gates the whole
// run — every repair is skipped with the sync-in-progress reason, nothing is
// written, and the still-held lock keeps exit 1.
func TestRunE_FixHeldLock_AllSkipped_ExitOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only; the windows gate path is covered by the seam tests")
	}

	tmp := t.TempDir()

	writeValidConfig(t, tmp)

	// A repairable problem (missing manifest) that must NOT be repaired.
	lockFile := filepath.Join(wapi.Dir(tmp), "sync.lock")

	require.NoError(t, os.WriteFile(lockFile, nil, 0o600))

	release := holdSyncLock(t, lockFile)

	defer release()

	c, out, _ := newTestCmd(t, "--dir", tmp, "--fix", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "the still-held lock keeps exit 1")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	require.Len(t, report.Actions, 3)

	for _, action := range report.Actions {
		assert.Equal(t, "skipped", action.Status, "action %s", action.ID)
		assert.Contains(t, action.Reason, "sync in progress", "action %s", action.ID)
	}

	_, statErr := os.Stat(filepath.Join(wapi.Dir(tmp), "manifest.json"))

	require.ErrorIs(t, statErr, os.ErrNotExist, "the manifest must NOT be rebuilt under a live sync")

	locked := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		locked[check.ID] = check
	}

	assert.Equal(t, "FAIL", locked["wapi.lock"].Status, "the post-fix suite still reports the held lock")
}

// TestRunE_FixRollbackRestoresFiles_TextActions covers VAL-FIX-006 at the
// command surface: the restore lands on disk and the text report shows the
// performed action.
func TestRunE_FixRollbackRestoresFiles_TextActions(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	backup := filepath.Join(wapi.Dir(tmp), ".rollback", "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(backup), 0o755))

	require.NoError(t, os.WriteFile(backup, []byte("backed up contents"), 0o600))

	working := filepath.Join(tmp, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("drifted contents"), 0o600))

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--fix")

	outStr := mustRun(t, c, out)

	assert.Empty(t, errOut.String())
	assert.Contains(t, outStr, "wapi.rollback: performed")

	restored, err := os.ReadFile(working)

	require.NoError(t, err)

	assert.Equal(t, "backed up contents", string(restored))

	_, statErr := os.Stat(filepath.Join(wapi.Dir(tmp), ".rollback"))

	assert.ErrorIs(t, statErr, os.ErrNotExist, ".rollback/ must be removed after the restore")
}

// TestRunE_FixRollbackRecreatesDeletedProjectFile covers VAL-FIX-007 end to
// end: a file the interrupted sync deleted comes back from the backup tree.
func TestRunE_FixRollbackRecreatesDeletedProjectFile(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	backup := filepath.Join(wapi.Dir(tmp), ".rollback", "app", "removed.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(backup), 0o755))

	require.NoError(t, os.WriteFile(backup, []byte("resurrected"), 0o600))

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--fix")

	require.NoError(t, c.Execute())

	restored, err := os.ReadFile(filepath.Join(tmp, "app", "removed.go"))

	require.NoError(t, err)

	assert.Equal(t, "resurrected", string(restored))
	assert.Contains(t, out.String(), "performed")
}

// TestRunE_FixSecondRunIsNoop covers VAL-FIX-012 at the command surface:
// after one successful fix, a second --fix run reports nothing to do and
// exits 0.
func TestRunE_FixSecondRunIsNoop(t *testing.T) {
	tmp := t.TempDir()

	writeValidConfig(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	first, _, _ := newTestCmd(t, "--dir", tmp, "--fix")

	require.NoError(t, first.Execute())

	second, out, _ := newTestCmd(t, "--dir", tmp, "--fix", "--output-format", "json")

	require.NoError(t, second.Execute())

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	assert.Equal(t, "ok", report.Status)

	for _, action := range report.Actions {
		assert.Equal(t, "not-needed", action.Status, "action %s", action.ID)
	}
}

// TestRunE_FixAndRelinkMutuallyExclusive covers the usage-error rule: --fix
// and --relink cannot be combined — the run errors with exit 1 and no checks
// run (the remote artifact seam is never called, stdout stays empty).
func TestRunE_FixAndRelinkMutuallyExclusive(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	calls := 0

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		calls++

		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--fix", "--relink", "abc123")

	require.Error(t, c.Execute(), "combining --fix and --relink must be a usage error")

	assert.Empty(t, out.String(), "no report may be rendered for a usage error")

	assert.Zero(t, calls, "no checks may run for a usage error")
}

// TestCmd_FixFlagShape pins the --fix flag's shape alongside the other flags.
func TestCmd_FixFlagShape(t *testing.T) {
	c := Cmd()

	fixFlag := c.Flags().Lookup("fix")

	require.NotNil(t, fixFlag)
	assert.Equal(t, "bool", fixFlag.Value.Type())
	assert.Equal(t, "false", fixFlag.DefValue)
}

// TestRunE_FixDeletedArtifactAndMissingManifest_LocalFixSucceedsRemoteStillFails
// covers VAL-FIX-020: the manifest rebuild (local) succeeds, but the deleted
// artifact (remote) remains FAIL — --fix is local-only and does not relink.
func TestRunE_FixDeletedArtifactAndMissingManifest_LocalFixSucceedsRemoteStillFails(t *testing.T) {
	tmp := t.TempDir()

	writeValidConfig(t, tmp)

	// No manifest.json: the rebuild will repair it.
	// The artifact is deleted (404): the remote check must remain FAIL.
	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "https://test/artifacts/x/"}
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--fix", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "the deleted artifact keeps exit 1 after the local fix")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON")

	assert.Equal(t, "fail", report.Status, "post-fix status is still fail (remote FAIL)")

	// The manifest rebuild was performed.
	require.NotEmpty(t, report.Actions)

	rebuild := report.Actions[0]

	assert.Equal(t, "wapi.manifest", rebuild.ID)
	assert.Equal(t, "performed", rebuild.Status)

	// The remote artifact-exists check is still FAIL with a relink remedy.
	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	exists := byID["remote.artifact-exists"]

	assert.Equal(t, "FAIL", exists.Status)
	assert.Contains(t, exists.Remedy, "--relink")

	// The local manifest check is now OK.
	manifest := byID["wapi.manifest"]

	assert.Equal(t, "OK", manifest.Status)
}
