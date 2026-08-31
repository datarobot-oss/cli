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
	"encoding/json"
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
	plan          *sync.SyncPlan
	planErr       error
	result        *sync.Result
	executeErr    error
	stale         bool
	migrationNote string
	ignoreNotice  string
	lockedNote    string
	fetcher       display.ContentFetcher
	closeErr      error

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

func (f *fakeEngine) StateMigrationNotice() string { return f.migrationNote }

func (f *fakeEngine) IgnoreFileNotice() string { return f.ignoreNotice }

func (f *fakeEngine) LockedNotice() string { return f.lockedNote }

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

// linkProject seeds a minimal state directory so the cmd's
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
// target directory has no state directory, with the expected hint pointing
// at `code init`.
func TestCmd_NotLinked(t *testing.T) {
	dir := t.TempDir()

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, _, err := runWithDeps(t, fakeEngineDeps(&fakeEngine{}), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

// TestRunE_DryRun_DoesNotPromptForDirectory: --dry-run writes nothing, so it
// resolves the directory without the prompt that used to block it forever
// before the plan (RAPTOR-19348). No --dir and no --yes: the old code prompted.
func TestRunE_DryRun_DoesNotPromptForDirectory(t *testing.T) {
	asked := false

	deps := fakeEngineDeps(&fakeEngine{plan: &sync.SyncPlan{}})
	deps.AskDir = func(_, defaultVal string) (string, error) {
		asked = true

		return defaultVal, nil
	}

	// The resolved "." is not a linked project, so the run ends in a "not
	// linked" error; the point of the test is that it got there without asking.
	_, _, _, _ = runWithDeps(t, deps, map[string]string{"dry-run": "true"})

	assert.False(t, asked, "--dry-run must not prompt for the project directory")
}

// TestRunE_Diff_DoesNotPromptForDirectory: --diff is the same no-write preview
// and must not block on the directory prompt either.
func TestRunE_Diff_DoesNotPromptForDirectory(t *testing.T) {
	asked := false

	deps := fakeEngineDeps(&fakeEngine{plan: &sync.SyncPlan{}})
	deps.AskDir = func(_, defaultVal string) (string, error) {
		asked = true

		return defaultVal, nil
	}

	_, _, _, _ = runWithDeps(t, deps, map[string]string{"diff": "true"})

	assert.False(t, asked, "--diff must not prompt for the project directory")
}

// TestRunE_Interactive_PromptsForDirectory is the control for the two preview
// tests above: a normal run with neither --dir nor --yes does reach the
// directory prompt, so those tests prove the preview modes skip it rather than
// the seam being dead.
func TestRunE_Interactive_PromptsForDirectory(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	asked := false

	deps := fakeEngineDeps(&fakeEngine{plan: &sync.SyncPlan{}})
	deps.AskDir = func(_, _ string) (string, error) {
		asked = true

		return dir, nil
	}

	_, _, _, err := runWithDeps(t, deps, map[string]string{})
	require.NoError(t, err)
	assert.True(t, asked, "an interactive run with no --dir must prompt for the directory")
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

// The engine lets a preview plan against a locked artifact, because the
// deploy needs the count. What the preview must never do is read as a sync
// that is going to work, so whatever the engine says about the lock has to
// reach the reader.
func TestRunE_LockedNoticeReachesStderr(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:       &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
		lockedNote: "Artifact art-1 is locked, so this is a preview only.",
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "is locked")
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

// The ignore-file notice is an advisory, not data. It belongs on stderr in
// every mode, and under --output-format json stdout has to stay parseable end
// to end, which is asserted by decoding it rather than by looking for the
// notice's absence: a substring check passes for any stdout that merely lacks
// that one sentence.
func TestRunE_IgnoreFileNotice_GoesToStderrAndLeavesStdoutParseable(t *testing.T) {
	const notice = "Using .wapiignore for sync ignore patterns."

	for _, format := range []string{"", "json"} {
		t.Run("output-format="+format, func(t *testing.T) {
			dir := t.TempDir()
			linkProject(t, dir)

			fe := &fakeEngine{
				plan:         &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
				result:       &sync.Result{NewVersion: "v2", UploadedCount: 1},
				ignoreNotice: notice,
			}

			flags := map[string]string{"dir": dir, "yes": "true"}
			if format != "" {
				flags["output-format"] = format
			}

			_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
			require.NoError(t, err)

			assert.Contains(t, stderr.String(), notice)
			assert.NotContains(t, stdout.String(), notice)

			if format == "json" {
				assertOnlyJSON(t, stdout)
			}
		})
	}
}

// assertOnlyJSON drains r and requires it to be exactly one JSON document. The
// command emits a single object — the plan, with the result nested under
// "result" when one ran — so a consumer's json.loads sees one document and no
// longer needs a splitter (RAPTOR-19348).
func assertOnlyJSON(t *testing.T, r io.Reader) {
	t.Helper()

	dec := json.NewDecoder(r)

	docs := 0

	for {
		var doc json.RawMessage

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}

		require.NoError(t, err, "stdout must contain only JSON documents")

		docs++
	}

	assert.Equal(t, 1, docs, "stdout must be exactly one JSON document")
}

// TestRunE_JSONOutput emits exactly one JSON document on the non-conflict,
// non-dry-run path: the plan at the top level with the executed result nested
// under "result", parseable in a single Unmarshal (RAPTOR-19348).
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

	assertOnlyJSON(t, bytes.NewReader(stdout.Bytes()))

	var doc struct {
		Uploads []struct {
			Path string `json:"path"`
		} `json:"uploads"`
		Result *struct {
			NewVersion string `json:"newVersion"`
		} `json:"result"`
	}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), "stdout must be one JSON object")
	require.Len(t, doc.Uploads, 1)
	assert.Equal(t, "a.py", doc.Uploads[0].Path)
	require.NotNil(t, doc.Result, "an executed run carries its result under \"result\"")
	assert.Equal(t, "v2", doc.Result.NewVersion)
}

// Exit is 0 and the upload list reads as a sync that will work, so this flag
// is the only thing in the document that says the plan can never be applied.
// The stderr notice does not reach a script.
func TestRunE_JSONOutput_LockedIsFlagged(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:       &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
		lockedNote: "Artifact art-1 is locked, so this is a preview only.",
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"locked": true`)
	assert.False(t, fe.executed)
}

// The ordinary case says so too, rather than leaving a reader to infer it from
// a key that is not there.
func TestRunE_JSONOutput_UnlockedIsFlagged(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}}}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"locked": false`)
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
