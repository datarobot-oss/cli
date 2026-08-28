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
