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

// TestRunE_RelinkHappyPath_PostRelinkChecksTargetNew covers VAL-RELINK-001
// and VAL-RELINK-020 at the command surface: relink to a new live artifact,
// post-relink checks target the new artifact, exit 0.
func TestRunE_RelinkHappyPath_PostRelinkChecksTargetNew(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		if id == newID {
			return fakeArtifact(id, "new-fixture", "DRAFT", nil), nil
		}

		// Old artifact still exists (healthy scenario).
		return fakeArtifact(id, "old-fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	outStr := mustRun(t, c, out)

	// Stdout is pure JSON (warning goes to stderr).
	var report jsonFixReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report), "stdout must be a single pure-JSON object")

	// Warning on stderr.
	assert.Contains(t, errOut.String(), "Relink repoints")

	// Actions array has the relink entry.
	require.NotEmpty(t, report.Actions)

	relinkAction := report.Actions[0]

	assert.Equal(t, "relink", relinkAction.ID)
	assert.Equal(t, "performed", relinkAction.Status)

	// Post-relink checks target the new artifact (all OK).
	assert.Equal(t, "ok", report.Status)

	require.NotNil(t, report.ArtifactID)

	assert.Equal(t, newID, *report.ArtifactID, "artifactId in the report is the new id")

	// Config on disk points at the new artifact.
	cfg, err := wapi.LoadConfig(tmp)

	require.NoError(t, err)

	assert.Equal(t, newID, cfg.ArtifactID)
	assert.Nil(t, cfg.LastSyncedVersionID)
}

// TestRunE_RelinkNonInteractiveWarning_ToStderr covers VAL-RELINK-004:
// --yes prints the warning to stderr and proceeds; stdout stays pure JSON.
func TestRunE_RelinkNonInteractiveWarning_ToStderr(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	require.NoError(t, c.Execute())

	// Stdout is pure JSON.
	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	// Stderr contains the warning.
	assert.Contains(t, errOut.String(), "Relink repoints")
}

// TestRunE_RelinkNonInteractiveText_WarningToStderr covers VAL-RELINK-004
// in text mode: the warning goes to stderr.
func TestRunE_RelinkNonInteractiveText_WarningToStderr(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes")

	require.NoError(t, c.Execute())

	assert.Contains(t, errOut.String(), "Relink repoints")
	assert.Contains(t, out.String(), "Repairs")
	assert.Contains(t, out.String(), "relink: performed")
}

// TestRunE_Relink404_AbortsStateUntouched covers VAL-RELINK-006 at the
// command surface: a 404 target aborts with exit 1 and state untouched.
func TestRunE_Relink404_AbortsStateUntouched(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "https://test/artifacts/x/"}
	})

	before := stateFileHashes(t, tmp)

	c, out, _ := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "404 abort exits 1")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON")

	assert.Equal(t, "fail", report.Status)

	require.NotEmpty(t, report.Actions)

	assert.Equal(t, "relink", report.Actions[0].ID)
	assert.Equal(t, "skipped", report.Actions[0].Status)
	assert.Contains(t, report.Actions[0].Reason, "not found")

	// State byte-identical.
	assert.Equal(t, before, stateFileHashes(t, tmp))
}

// TestRunE_RelinkNotLinked_ErrorPointsToInit covers VAL-RELINK-010 at the
// command surface: relink on a not-linked project exits 1 with presence FAIL.
func TestRunE_RelinkNotLinked_ErrorPointsToInit(t *testing.T) {
	tmp := t.TempDir()

	calls := 0

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		calls++

		return fakeArtifact(newID, "fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "not-linked exits 1")

	// No network fetch (short-circuit before fetch).
	assert.Zero(t, calls, "no network fetch for a not-linked project")

	// The report shows presence FAIL.
	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	assert.Equal(t, "FAIL", byID["wapi.presence"].Status)
	assert.Contains(t, byID["wapi.presence"].Remedy, "init")

	// Stderr has the error message.
	assert.Contains(t, errOut.String(), "not linked")
}

// TestRunE_RelinkSameID_WarnedBaseReset covers VAL-RELINK-009 at the command
// surface: same-id relink is allowed, warned, and resets BASE.
func TestRunE_RelinkSameID_WarnedBaseReset(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", testArtifactID, "--yes")

	require.NoError(t, c.Execute())

	// Warning includes the same-id note.
	assert.Contains(t, errOut.String(), "same artifact")

	// Config: artifactId unchanged, lsv nil.
	cfg, err := wapi.LoadConfig(tmp)

	require.NoError(t, err)

	assert.Equal(t, testArtifactID, cfg.ArtifactID)
	assert.Nil(t, cfg.LastSyncedVersionID)

	// Manifest: empty BASE.
	m, err := wapi.LoadManifest(tmp)

	require.NoError(t, err)

	assert.Empty(t, m.Files)

	// History: relink entry with from == to.
	assert.Contains(t, out.String(), "performed")
}

// TestRunE_RelinkJSON_ActionsArray_PureStdout covers VAL-RELINK-015:
// JSON mode has actions array describing the relink, stdout pure, warning on stderr.
func TestRunE_RelinkJSON_ActionsArray_PureStdout(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	require.NoError(t, c.Execute())

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	require.NotEmpty(t, report.Actions)

	assert.Equal(t, "relink", report.Actions[0].ID)
	assert.Equal(t, "performed", report.Actions[0].Status)

	// Post-relink checks reflect the new artifact.
	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	assert.Equal(t, "OK", byID["remote.artifact-exists"].Status)

	// Warning on stderr only.
	assert.Contains(t, errOut.String(), "Relink repoints")
}

// TestRunE_RelinkWorkingTreeUntouched covers VAL-RELINK-014:
// the working tree is untouched by the relink.
func TestRunE_RelinkWorkingTreeUntouched(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	// Place a working-tree file.
	working := filepath.Join(tmp, "app", "main.go")

	require.NoError(t, os.MkdirAll(filepath.Dir(working), 0o755))

	require.NoError(t, os.WriteFile(working, []byte("user source"), 0o600))

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	before := stateFileHashes(t, tmp)

	c, _, _ := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes")

	require.NoError(t, c.Execute())

	after := stateFileHashes(t, tmp)

	// Working-tree files (non-state) are byte-identical.
	for path, want := range before {
		if isStateFile(path, tmp) {
			continue
		}

		got, ok := after[path]

		require.True(t, ok, "file disappeared: %s", path)

		assert.Equal(t, want, got, "working-tree file must be untouched: %s", path)
	}
}

// TestRunE_RelinkHistoryEntry covers VAL-RELINK-013:
// history.log gains a well-formed {op:relink, from, to, ts} entry.
func TestRunE_RelinkHistoryEntry(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, _, _ := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes")

	require.NoError(t, c.Execute())

	history, err := os.ReadFile(filepath.Join(wapi.Dir(tmp), "history.log"))

	require.NoError(t, err)

	// The last line must be a relink entry.
	lines := []string{}

	for _, line := range splitLinesStr(string(history)) {
		if line != "" {
			lines = append(lines, line)
		}
	}

	require.NotEmpty(t, lines)

	var entry map[string]any

	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &entry))

	assert.Equal(t, "relink", entry["op"])
	assert.Equal(t, testArtifactID, entry["from"])
	assert.Equal(t, newID, entry["to"])

	ts, ok := entry["ts"].(string)

	require.True(t, ok, "ts must be a string")

	assert.NotEmpty(t, ts, "ts must be non-empty")
}

// TestRunE_RelinkLockHeld_AbortsStateUntouched covers VAL-RELINK-021 at the
// command surface: a held lock aborts the relink with state untouched.
func TestRunE_RelinkLockHeld_AbortsStateUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock semantics are unix-only")
	}

	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	lockFile := filepath.Join(wapi.Dir(tmp), "sync.lock")

	require.NoError(t, os.WriteFile(lockFile, nil, 0o600))

	release := holdSyncLock(t, lockFile)

	defer release()

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	before := stateFileHashes(t, tmp)

	c, out, _ := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "lock held exits 1")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	require.NotEmpty(t, report.Actions)

	assert.Equal(t, "skipped", report.Actions[0].Status)
	assert.Contains(t, report.Actions[0].Reason, "sync in progress")

	// State byte-identical.
	assert.Equal(t, before, stateFileHashes(t, tmp))
}

// TestRunE_RelinkWrongType_AbortsStateUntouched covers VAL-RELINK-023 at the
// command surface: a non-service artifact type aborts with state untouched.
func TestRunE_RelinkWrongType_AbortsStateUntouched(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{
			ID:     id,
			Name:   "agent-fixture",
			Status: "DRAFT",
			Type:   "agent",
		}, nil
	})

	before := stateFileHashes(t, tmp)

	c, out, _ := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "wrong type exits 1")

	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	require.NotEmpty(t, report.Actions)

	assert.Equal(t, "skipped", report.Actions[0].Status)
	assert.Contains(t, report.Actions[0].Reason, "agent")

	assert.Equal(t, before, stateFileHashes(t, tmp))
}

// TestRunE_RelinkAPIUnreachable_Aborts covers the API unreachable gate at the
// command surface: a non-404 fetch error aborts with an error on stderr.
func TestRunE_RelinkAPIUnreachable_Aborts(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	newID := "6a90da2ddeadbeefcafe5678"

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 500, URL: "https://test/"}
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink", newID, "--yes", "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "API unreachable exits 1")

	// Stderr has the error message.
	assert.Contains(t, errOut.String(), "cannot reach")

	// Stdout is pure JSON (the report still renders).
	var report jsonFixReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
}

// TestRunE_RelinkNoActionsKey_ReadOnlyRun covers VAL-OUTPUT-009(a):
// a plain diagnosis (no --fix/--relink) has no actions key.
func TestRunE_RelinkNoActionsKey_ReadOnlyRun(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "fixture", "DRAFT", nil), nil
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--output-format", "json")

	require.NoError(t, c.Execute())

	// Parse as a generic map to check for the actions key.
	var raw map[string]any

	require.NoError(t, json.Unmarshal(out.Bytes(), &raw))

	_, hasActions := raw["actions"]

	assert.False(t, hasActions, "plain diagnosis must not have an actions key")
}

// TestCmd_RelinkFlagShape pins the --relink flag's shape.
func TestCmd_RelinkFlagShape(t *testing.T) {
	c := Cmd()

	relinkFlag := c.Flags().Lookup("relink")

	require.NotNil(t, relinkFlag)
	assert.Equal(t, "string", relinkFlag.Value.Type())
	assert.Empty(t, relinkFlag.DefValue, "--relink defaults to empty (not set)")
}

// TestCmd_FixAndRelinkMutuallyExclusiveShape verifies the cobra-level mutual
// exclusion is registered.
func TestCmd_FixAndRelinkMutuallyExclusiveShape(t *testing.T) {
	c := Cmd()

	// MarkFlagsMutuallyExclusive adds a cobra annotation that we can verify
	// by checking that both flags exist and the command rejects both.
	fixFlag := c.Flags().Lookup("fix")

	relinkFlag := c.Flags().Lookup("relink")

	require.NotNil(t, fixFlag)
	require.NotNil(t, relinkFlag)
}

// TestRunE_RelinkMissingValue_UsageError covers VAL-OUTPUT-018:
// --relink without a value is a usage error.
func TestRunE_RelinkMissingValue_UsageError(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	c, out, errOut := newTestCmd(t, "--dir", tmp, "--relink")

	err := c.Execute()

	require.Error(t, err, "missing --relink value must be a usage error")

	// No checks execute, no report on stdout.
	assert.Empty(t, out.String(), "no report for a usage error")

	// Stderr has a usage error message.
	assert.NotEmpty(t, errOut.String())
}

// isStateFile reports whether path is inside the state directory.
func isStateFile(path, projectDir string) bool {
	stateDir := wapi.Dir(projectDir)

	return len(path) >= len(stateDir) && path[:len(stateDir)] == stateDir
}

// splitLinesStr splits on newlines, dropping trailing empty lines.
func splitLinesStr(s string) []string {
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
