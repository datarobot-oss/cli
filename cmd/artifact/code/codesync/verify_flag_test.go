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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCmd_VerifyFlagRegistered pins the --verify registration: a boolean
// flag whose usage string tells the user what the flag buys them — the
// forced remote round-trip and the post-apply verification of what was
// uploaded. A flag that is present but unexplained reads as plumbing to
// nobody, so the usage string is part of the contract.
func TestCmd_VerifyFlagRegistered(t *testing.T) {
	cmd := Cmd()

	flag := cmd.Flags().Lookup("verify")
	require.NotNil(t, flag, "--verify must be registered on the sync command")
	assert.Equal(t, "bool", flag.Value.Type())
	assert.NotEmpty(t, flag.Usage, "usage string must be non-empty")
	assert.Contains(t, flag.Usage, "round-trip", "usage must describe the forced remote round-trip")
	assert.Contains(t, flag.Usage, "verif", "usage must describe the post-apply verification of what was uploaded")

	verified, err := cmd.Flags().GetBool("verify")
	require.NoError(t, err)
	assert.False(t, verified, "--verify must default to false")
}

// TestCmd_VerifyComposesWithOtherFlags: --verify gates network-cost checks,
// not rendering or confirmation, so it must parse alongside every other
// flag the command accepts — in particular it must NOT join the
// dry-run/diff mutual-exclusion group, and no combination here may be
// rejected by the parser.
func TestCmd_VerifyComposesWithOtherFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--verify", "--dry-run", "--yes"},
		{"--verify", "--diff", "--yes"},
		{"--verify", "--yes", "--output-format", "json"},
	} {
		cmd := Cmd()

		require.NoError(t, cmd.ParseFlags(args), "flag set %v must be accepted with no flag-conflict error", args)

		verified, err := cmd.Flags().GetBool("verify")
		require.NoError(t, err)

		assert.True(t, verified, "--verify must parse as true for args %v", args)
	}
}

// capturingEngineDeps returns Deps whose NewEngine records the options it
// was called with before handing back fe, so a test can assert the flag
// actually reached the engine rather than stopping at the command layer.
func capturingEngineDeps(fe *fakeEngine, captured *sync.Options) Deps {
	return Deps{
		NewEngine: func(_ string, opts sync.Options) (engineRunner, error) {
			*captured = opts

			return fe, nil
		},
	}
}

// TestCmd_VerifyThreadedIntoEngineOptions: the flag value must arrive in
// sync.Options.Verify — set with --verify, clear without it — for a test
// to catch the value stopping anywhere between cobra and the engine.
func TestCmd_VerifyThreadedIntoEngineOptions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extraFlag string
		want      bool
	}{
		{name: "with --verify", extraFlag: "verify", want: true},
		{name: "without --verify", extraFlag: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			linkProject(t, dir)

			fe := &fakeEngine{
				plan:   &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
				result: &sync.Result{NewVersion: "v2", UploadedCount: 1},
			}

			var captured sync.Options

			flags := map[string]string{"dir": dir, "yes": "true"}
			if tc.extraFlag != "" {
				flags[tc.extraFlag] = "true"
			}

			_, _, _, err := runWithDeps(t, capturingEngineDeps(fe, &captured), flags)
			require.NoError(t, err)
			assert.Equal(t, tc.want, captured.Verify, "Options.Verify must mirror the --verify flag")
		})
	}
}

// TestCmd_VerifyStillExecutes_NonDryRun: --verify is a diagnostic switch,
// not a preview mode. A non-dry-run verify run must reach Execute, create
// its version, and print the completion summary — the failure mode this
// guards is --verify being absorbed into the preview-only predicate and
// silently turning every verify user's sync into a preview.
func TestCmd_VerifyStillExecutes_NonDryRun(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:   &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
		result: &sync.Result{NewVersion: "v2", UploadedCount: 1},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.True(t, fe.executed, "a non-dry-run --verify run must still Execute the plan")
	assert.Contains(t, stdout.String(), "Sync complete", "the completion summary must be printed")
}

// sampleDivergences is the flagship-shaped finding set: a hash mismatch plus
// both one-sided kinds, so every test below exercises a notice that carries
// all three distinctions.
func sampleDivergences() []sync.Divergence {
	return []sync.Divergence{
		{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		{Path: "gone.py", Kind: sync.DivergenceBaseOnly, BaseHash: "cccc"},
		{Path: "stray.py", Kind: sync.DivergenceRemoteOnly, RemoteHash: "dddd"},
	}
}

// The divergence notice is a diagnostic, and diagnostics never belong on the
// data stream: it must reach stderr in human mode and stay off stdout.
func TestRunE_DivergenceNotice_ReachesStderrOnly(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:        &sync.SyncPlan{},
		divergences: sampleDivergences(),
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "verify": "true"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	errText := stderr.String()

	// The summary names every divergent path, not just the first.
	assert.Contains(t, errText, "app.py")
	assert.Contains(t, errText, "gone.py")
	assert.Contains(t, errText, "stray.py")

	assert.NotContains(t, stdout.String(), "app.py", "divergence prose must stay off the data stream")
}

// Under --output-format json the divergence data flows into the plan document
// as a structured field, and every trace of the prose stays on stderr so
// stdout decodes as JSON to EOF.
func TestRunE_JSON_DivergenceFieldPopulated(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:        &sync.SyncPlan{},
		divergences: sampleDivergences(),
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "verify": "true", "output-format": "json"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	// Capture before any reader drains the buffer: assertOnlyJSON decodes to
	// EOF, and a bytes.Buffer stays empty afterwards.
	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))

	assert.Contains(t, stderr.String(), "app.py", "the divergence notice must remain on stderr in JSON mode")

	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		Divergence []struct {
			Path       string `json:"path"`
			Kind       string `json:"kind"`
			BaseHash   string `json:"baseHash"`
			RemoteHash string `json:"remoteHash"`
		} `json:"divergence"`
	}

	require.NoError(t, dec.Decode(&doc), "the first stdout document must be the plan")
	require.Len(t, doc.Divergence, 3, "every divergent path must be listed in the plan document")

	assert.Equal(t, "app.py", doc.Divergence[0].Path)
	assert.Equal(t, "hash_mismatch", doc.Divergence[0].Kind)
	assert.Equal(t, "aaaa", doc.Divergence[0].BaseHash)
	assert.Equal(t, "bbbb", doc.Divergence[0].RemoteHash)

	assert.Equal(t, "gone.py", doc.Divergence[1].Path)
	assert.Equal(t, "base_only", doc.Divergence[1].Kind)

	assert.Equal(t, "stray.py", doc.Divergence[2].Path)
	assert.Equal(t, "remote_only", doc.Divergence[2].Kind)
}

// A healthy --verify run leaves the field explicitly empty rather than
// omitted: a script must be able to tell "no divergence" from "the field was
// never emitted" (the locked:false precedent).
func TestRunE_JSON_DivergenceExplicitlyEmptyWhenClean(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "verify": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	// Same capture-before-drain discipline as above.
	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))
	assert.Contains(t, raw, `"divergence": []`)
}

// The empty-plan repair: a verify run whose plan came back empty but whose
// divergence findings are non-empty must still reach Execute. The engine's
// Phase 5 no-ops on an empty plan, so the Execute is what drives the Phase 6
// state write that rewrites the poisoned manifest from the real remote;
// skipping Execute here is how the poison survives the run that caught it.
func TestRunE_EmptyPlan_WithDivergence_ExecutesForRepair(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
		result: &sync.Result{NewVersion: "v1"},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.True(t, fe.executed,
		"an empty plan with divergence findings must still Execute so Phase 6 repairs the manifest")
	assert.Contains(t, stderr.String(), "app.py", "the divergence summary must still reach stderr")
}

// JSON-mode twin of the repair path: the plan document is emitted first (its
// divergence field populated), then Execute runs and the result document
// follows — the same two-document shape as a normal applying sync.
func TestRunE_JSON_EmptyPlan_WithDivergence_ExecutesAndEmitsResult(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
		result: &sync.Result{NewVersion: "v1"},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))
	assert.True(t, fe.executed, "the JSON path must Execute for the empty-plan repair too")
	assert.Contains(t, raw, `"v1"`, "the result document must follow the plan document")
}

// The one-line divergence summary must not claim a reconciliation the run
// does not perform. In a preview (--dry-run/--diff) nothing is written at
// all, so the summary has to say so and point at the mode flag instead of
// asserting that the plan reconciles anything.
func TestRunE_DivergenceSummary_PreviewSaysNothingWritten(t *testing.T) {
	for _, tc := range []struct {
		name        string
		previewFlag string
		runWithout  string
	}{
		{name: "--dry-run", previewFlag: "dry-run", runWithout: "--dry-run"},
		{name: "--diff", previewFlag: "diff", runWithout: "--diff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			linkProject(t, dir)

			fe := &fakeEngine{
				plan:        &sync.SyncPlan{},
				divergences: sampleDivergences(),
			}

			flags := map[string]string{"dir": dir, "yes": "true", "verify": "true", tc.previewFlag: "true"}

			_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
			require.NoError(t, err)

			errText := stderr.String()

			assert.Contains(t, errText, "This was a preview; nothing was written.",
				"a preview must not read as a run that reconciles anything")
			assert.Contains(t, errText, fmt.Sprintf("Run without %s to reconcile.", tc.runWithout),
				"the summary must say how to actually reconcile")

			assert.NotContains(t, errText, "The plan reconciles them.",
				"nothing is reconciled in a preview; the applying wording must not appear")
		})
	}
}

// The empty-plan repair run reconciles nothing through plan rows — the plan
// has none. The reconciliation is the Phase 6 manifest rewrite, so the
// summary must say the manifest is being rewritten from the server's state
// rather than repeating "The plan reconciles them.".
func TestRunE_DivergenceSummary_EmptyPlanRepair_SaysManifestRewritten(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:        &sync.SyncPlan{},
		divergences: sampleDivergences(),
		result:      &sync.Result{NewVersion: "v1"},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	errText := stderr.String()

	assert.True(t, fe.executed, "the repair Execute must still run")
	assert.Contains(t, errText,
		"The plan is empty, but manifest.json is being rewritten from the server's state",
		"the summary must name the manifest rewrite, not plan rows")
	assert.NotContains(t, errText, "The plan reconciles them.",
		"an empty plan reconciles no rows; the applying wording would mislead")
}

// The ordinary applying wording is load-bearing and unchanged: a non-empty
// plan really does reconcile the divergences through its rows.
func TestRunE_DivergenceSummary_ApplyingKeepsReconcilesWording(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:        &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "app.py"}}},
		divergences: sampleDivergences(),
		result:      &sync.Result{NewVersion: "v2", UploadedCount: 1},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.True(t, fe.executed)
	assert.Contains(t, stderr.String(), "The plan reconciles them.",
		"the non-empty applying wording must stay as it is")
}

// The empty-plan repair run prints its honest line to stdout in place of
// "Up to date.": the plan really is empty, but the run is about to repair
// the manifest, and claiming the project is up to date immediately before
// that rewrite is the lie scrutiny flagged. The completion summary from the
// state write must still follow.
func TestRunE_EmptyPlanRepair_NeverSaysUpToDate(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
		result: &sync.Result{NewVersion: "v1"},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	out := stdout.String()

	assert.NotContains(t, out, "Up to date.",
		"the repair run must not claim the project is up to date")
	assert.Contains(t, out,
		"The plan is empty, but manifest.json is being rewritten from the server's state to repair the divergences found by --verify.",
		"stdout must say the plan is empty and the manifest is being repaired")
	assert.Contains(t, out, "Sync complete",
		"the state-write completion summary must still follow")
}

// The genuinely converged case keeps the pre-existing line: with no
// divergence findings there is nothing to repair, the empty plan
// short-circuits before Execute, and "Up to date." is the truth.
func TestRunE_EmptyPlan_Converged_KeepsUpToDate(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}}

	flags := map[string]string{"dir": dir, "yes": "true", "verify": "true"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.False(t, fe.executed, "no divergences means the empty-plan short-circuit stands")
	assert.Contains(t, stdout.String(), "Up to date.",
		"a genuinely converged empty plan must keep its existing line")
}

// The summary is keyed off divergence findings alone: with none, every mode
// and plan shape must render nothing, so a plain sync gains no extra prose.
func TestDivergenceSummaryNotice_EmptyWhenNoDivergences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags runFlags
		plan  *sync.SyncPlan
	}{
		{name: "applying", flags: runFlags{}, plan: &sync.SyncPlan{}},
		{name: "applying non-empty", flags: runFlags{}, plan: &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}}},
		{name: "dry-run", flags: runFlags{DryRun: true}, plan: &sync.SyncPlan{}},
		{name: "diff", flags: runFlags{Diff: true}, plan: &sync.SyncPlan{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, divergenceSummaryNotice(tc.flags, tc.plan, nil))
		})
	}
}
