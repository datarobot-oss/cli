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

package syncc

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/sync/display"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEngine is a stand-in for *sync.Engine. Tests configure the
// canned Plan/Execute responses up front and inspect the executed
// flag to assert the cmd's branch decisions (dry-run, diff, conflict
// prompt, JSON output).
type fakeEngine struct {
	plan       *sync.SyncPlan
	planErr    error
	result     *sync.Result
	executeErr error
	stale      bool
	fetcher    display.ContentFetcher
	closeErr   error

	executed bool
	closed   bool
}

func (f *fakeEngine) Plan() (*sync.SyncPlan, error) { return f.plan, f.planErr }

func (f *fakeEngine) Execute(_ *sync.SyncPlan) (*sync.Result, error) {
	f.executed = true

	return f.result, f.executeErr
}

func (f *fakeEngine) Close() error {
	f.closed = true

	return f.closeErr
}

func (f *fakeEngine) StaleRollbackRestored() bool { return f.stale }

func (f *fakeEngine) Fetcher() display.ContentFetcher { return f.fetcher }

// withFakeEngine installs fe behind newEngine for the duration of the
// test, restoring the original constructor on cleanup.
func withFakeEngine(t *testing.T, fe *fakeEngine) {
	t.Helper()

	orig := newEngine
	newEngine = func(_ string, _ sync.Options) (engineRunner, error) {
		return fe, nil
	}

	t.Cleanup(func() { newEngine = orig })
}

// withStubReader replaces promptReadLine so the conflict menu reads
// deterministic input. After the lines are exhausted, returns io.EOF
// (which the menu treats as quit).
func withStubReader(t *testing.T, lines ...string) {
	t.Helper()

	orig := promptReadLine
	i := 0
	promptReadLine = func() (string, error) {
		if i >= len(lines) {
			return "", io.EOF
		}

		line := lines[i]
		i++

		return line + "\n", nil
	}

	t.Cleanup(func() { promptReadLine = orig })
}

// linkProject seeds a minimal .wapi/ directory so the cmd's
// "not linked" preflight passes.
func linkProject(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-test-001"}))
}

// runWithFlags wires up Cmd() with PreRunE disabled (so tests don't
// go through auth) and the given flag set. Returns the captured
// stdout/stderr and the error from cmd.Execute.
func runWithFlags(t *testing.T, flags map[string]string, extraArgs ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}

	cmd.SetArgs(extraArgs)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := cmd.Execute()

	return cmd, stdout, stderr, err
}

// TestCmd_NotLinked confirms the command refuses to run when the
// target directory has no .wapi/, with the expected hint pointing at
// `code init`.
func TestCmd_NotLinked(t *testing.T) {
	dir := t.TempDir()

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

// TestCmd_DryRunDiffMutuallyExclusive confirms cobra's
// MarkFlagsMutuallyExclusive wiring is in place.
func TestCmd_DryRunDiffMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "diff": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[dry-run diff]")
}

// TestRunE_EmptyPlan_NoExecute: empty plan short-circuits before
// Execute and Close runs via defer.
func TestRunE_EmptyPlan_NoExecute(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir, "yes": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.False(t, fe.executed, "Execute must not run on empty plan")
	assert.True(t, fe.closed, "Close must run via defer")
}

// TestRunE_DryRun_NonEmpty_NoExecute: --dry-run on a non-empty plan
// must skip Execute.
func TestRunE_DryRun_NonEmpty_NoExecute(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{
		Uploads: []sync.FileAction{{Path: "a.py"}},
	}}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.False(t, fe.executed)
}

// TestRunE_PlanError_Propagates: errors from Plan surface verbatim.
func TestRunE_PlanError_Propagates(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	upstream := errors.New("phase-3 boom")
	withFakeEngine(t, &fakeEngine{planErr: upstream})

	flags := map[string]string{"dir": dir, "yes": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream)
}

// TestRunE_JSONOutput emits the plan plus result as two JSON
// documents on the non-conflict, non-dry-run path.
func TestRunE_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	plan := &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}}
	result := &sync.Result{NewVersion: "v2", UploadedCount: 1}
	fe := &fakeEngine{plan: plan, result: result}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir, "yes": "true", "output-format": "json"}

	_, stdout, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "non-dry-run JSON path must Execute")

	out := stdout.String()
	assert.Contains(t, out, `"a.py"`)
	assert.Contains(t, out, `"v2"`)
}

// TestRunE_ConflictPromptQuit: with conflicts present and no --yes,
// typing 'q' aborts cleanly without calling Execute.
func TestRunE_ConflictPromptQuit(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{
		Conflicts: []sync.FileAction{{Path: "x.py"}},
	}}
	withFakeEngine(t, fe)
	withStubReader(t, "q")

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.False(t, fe.executed, "user typed q; Execute must not run")
}

// TestRunE_ConflictPromptSync: with conflicts present and no --yes,
// pressing Enter accepts the plan and Execute runs.
func TestRunE_ConflictPromptSync(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:   &sync.SyncPlan{Conflicts: []sync.FileAction{{Path: "x.py"}}},
		result: &sync.Result{NewVersion: "v3", ConflictCount: 1},
	}
	withFakeEngine(t, fe)
	withStubReader(t, "")

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "user pressed Enter; Execute must run")
}

// TestRunE_NonInteractiveEnvVar: DATAROBOT_CLI_NON_INTERACTIVE=true
// skips the conflict prompt entirely (no reader stub installed; if
// the prompt were reached the test would hang or read os.Stdin).
func TestRunE_NonInteractiveEnvVar(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "true")

	fe := &fakeEngine{
		plan:   &sync.SyncPlan{Conflicts: []sync.FileAction{{Path: "x.py"}}},
		result: &sync.Result{NewVersion: "v3"},
	}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.True(t, fe.executed)
}

// TestRunE_StaleRollbackHint: when the engine reports a recovered
// stale rollback, the cmd writes the recovery line to stderr.
func TestRunE_StaleRollbackHint(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}, stale: true}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, stderr, err := runWithFlags(t, flags)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "Recovered from interrupted sync")
}

// TestRunE_CloseError_NotSurfaced: a Close() error from the engine
// (e.g., lock release failure) is logged at the engine layer and
// debug-logged at the cmd layer, but must not surface as a cmd error.
func TestRunE_CloseError_NotSurfaced(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}, closeErr: errors.New("lock release failed")}
	withFakeEngine(t, fe)

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithFlags(t, flags)
	require.NoError(t, err, "Close errors must be swallowed at the cmd boundary")
	assert.True(t, fe.closed)
}
