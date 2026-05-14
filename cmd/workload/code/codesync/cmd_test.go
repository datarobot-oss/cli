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

package codesync

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

// fakeEngineDeps returns a Deps that hands fe back from NewEngine and
// has no reader configured. Tests that need to drive the conflict menu
// override ReadLine via stubReader.
func fakeEngineDeps(fe *fakeEngine) Deps {
	return Deps{
		NewEngine: func(_ string, _ sync.Options) (engineRunner, error) {
			return fe, nil
		},
	}
}

// stubReader returns a ReadLine func that yields the given lines in
// order, then io.EOF (which the conflict menu treats as quit).
func stubReader(lines ...string) func() (string, error) {
	i := 0

	return func() (string, error) {
		if i >= len(lines) {
			return "", io.EOF
		}

		line := lines[i]
		i++

		return line + "\n", nil
	}
}

// linkProject seeds a minimal .wapi/ directory so the cmd's
// "not linked" preflight passes.
func linkProject(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-test-001"}))
}

// runWithDeps wires up cmdWithDeps(deps) with PreRunE disabled (so
// tests don't go through auth) and the given flag set. Returns the
// captured stdout/stderr and the error from cmd.Execute.
func runWithDeps(t *testing.T, deps Deps, flags map[string]string, extraArgs ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	cmd := cmdWithDeps(deps)
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

	_, _, _, err := runWithDeps(t, fakeEngineDeps(&fakeEngine{}), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

// TestCmd_DryRunDiffMutuallyExclusive confirms cobra's
// MarkFlagsMutuallyExclusive wiring is in place.
func TestCmd_DryRunDiffMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "diff": "true"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(&fakeEngine{}), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[dry-run diff]")
}

// TestRunE_EmptyPlan_NoExecute: empty plan short-circuits before
// Execute and Close runs via defer.
func TestRunE_EmptyPlan_NoExecute(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}}

	flags := map[string]string{"dir": dir, "yes": "true"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
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

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.False(t, fe.executed)
}

// TestRunE_PlanError_Propagates: errors from Plan surface verbatim.
func TestRunE_PlanError_Propagates(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	upstream := errors.New("phase-3 boom")
	deps := fakeEngineDeps(&fakeEngine{planErr: upstream})

	flags := map[string]string{"dir": dir, "yes": "true"}

	_, _, _, err := runWithDeps(t, deps, flags)
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

	flags := map[string]string{"dir": dir, "yes": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "non-dry-run JSON path must Execute")

	out := stdout.String()
	assert.Contains(t, out, `"a.py"`)
	assert.Contains(t, out, `"v2"`)
}

// TestRunE_JSONOutput_ConflictWithoutYes: with conflicts present and
// no --yes, the JSON path must emit the plan and stop — it must NOT
// auto-execute. Mirrors the human-path quit branch and prevents a
// silent overwrite when scripts pass --output-format json without
// realising the plan has conflicts.
func TestRunE_JSONOutput_ConflictWithoutYes(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{
		Conflicts: []sync.FileAction{{Path: "x.py"}},
	}}

	flags := map[string]string{"dir": dir, "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.False(t, fe.executed, "JSON path must not auto-execute on conflicts without --yes")
	assert.Contains(t, stdout.String(), `"x.py"`, "plan JSON should still be emitted")
}

// TestRunE_JSONOutput_ConflictWithYes: --yes opts into auto-execute
// even on conflicts in JSON mode, matching the human-path Enter branch.
func TestRunE_JSONOutput_ConflictWithYes(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:   &sync.SyncPlan{Conflicts: []sync.FileAction{{Path: "x.py"}}},
		result: &sync.Result{NewVersion: "v3", ConflictCount: 1},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "output-format": "json"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "--yes must allow auto-execute on conflicts")
}

// TestRunE_ConflictPromptQuit: with conflicts present and no --yes,
// typing 'q' aborts cleanly without calling Execute.
func TestRunE_ConflictPromptQuit(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{
		Conflicts: []sync.FileAction{{Path: "x.py"}},
	}}
	deps := fakeEngineDeps(fe)
	deps.ReadLine = stubReader("q")

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithDeps(t, deps, flags)
	require.NoError(t, err)
	assert.False(t, fe.executed, "user typed q; Execute must not run")
}

// TestRunE_ConflictPromptEOF: when stdin closes mid-prompt (Ctrl+D
// or piped input ran out), the menu treats it as a clean quit — no
// "Error: EOF" surfaces, matching reader.AskYesNo convention.
func TestRunE_ConflictPromptEOF(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{
		Conflicts: []sync.FileAction{{Path: "x.py"}},
	}}
	deps := fakeEngineDeps(fe)
	deps.ReadLine = stubReader() // no lines → first call returns io.EOF

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithDeps(t, deps, flags)
	require.NoError(t, err, "EOF must not surface as a cmd error")
	assert.False(t, fe.executed, "EOF on prompt = clean quit, no Execute")
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
	deps := fakeEngineDeps(fe)
	deps.ReadLine = stubReader("")

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithDeps(t, deps, flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "user pressed Enter; Execute must run")
}

// stubFetcher is a tiny display.ContentFetcher used by the "show diffs
// then sync" test below — it returns canned bytes for the seeded conflict
// path so PrintDiffs has something to render between prompts.
type stubFetcher struct {
	local, remote map[string]string
}

func (s *stubFetcher) LocalContent(path string) ([]byte, error) { return []byte(s.local[path]), nil }

func (s *stubFetcher) RemoteContent(path string) ([]byte, error) {
	return []byte(s.remote[path]), nil
}

// TestRunE_ConflictPromptShowDiffsThenSync exercises the `d` → re-prompt
// loop: typing 'd' renders per-file diffs and stays in the prompt; the
// next Enter accepts the plan and Execute runs.
func TestRunE_ConflictPromptShowDiffsThenSync(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fetcher := &stubFetcher{
		local:  map[string]string{"x.py": "AAAA\n"},
		remote: map[string]string{"x.py": "ZZZZ\n"},
	}
	fe := &fakeEngine{
		plan: &sync.SyncPlan{Conflicts: []sync.FileAction{
			{Path: "x.py", Classification: sync.ClsConflict, Action: sync.ActConflictCopy},
		}},
		result:  &sync.Result{NewVersion: "v3", ConflictCount: 1},
		fetcher: fetcher,
	}
	deps := fakeEngineDeps(fe)
	deps.ReadLine = stubReader("d", "")

	flags := map[string]string{"dir": dir}

	_, stdout, _, err := runWithDeps(t, deps, flags)
	require.NoError(t, err)
	assert.True(t, fe.executed, "Enter after 'd' must run Execute")
	assert.Contains(t, stdout.String(), "x.py", "diff render must mention the conflict path")
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

	flags := map[string]string{"dir": dir}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.True(t, fe.executed)
}

// TestRunE_StaleRollbackHint: when the engine reports a recovered
// stale rollback, the cmd writes the recovery line to stderr.
func TestRunE_StaleRollbackHint(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}, stale: true}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
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

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err, "Close errors must be swallowed at the cmd boundary")
	assert.True(t, fe.closed)
}
