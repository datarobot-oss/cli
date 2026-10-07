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

package up

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/up"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCmd_DiffIsRegistered keeps the flag discoverable: cobra lists it in
// --help only when it is registered, not hidden, and defaulted off, and the
// help text is what tells a reader what they are about to get.
func TestCmd_DiffIsRegistered(t *testing.T) {
	lookup := Cmd().Flags().Lookup("diff")

	require.NotNil(t, lookup, "the flag has to exist for --diff to parse at all")
	assert.False(t, lookup.Hidden)
	assert.Equal(t, "false", lookup.DefValue, "the diff rendering is opt-in")
	assert.Contains(t, lookup.Usage, "unified diff")
}

// TestCmd_DiffReachesTheDeploy threads the flag into the deploy's options,
// and only when it was given: a run without it must not silently start
// rendering diffs.
func TestCmd_DiffReachesTheDeploy(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--dry-run", "--diff")
	require.NoError(t, err)
	assert.True(t, seen.Diff)
	assert.True(t, seen.DryRun)

	defaults := stubRun(t, deployed(), nil)

	_, _, err = runCmd(t)
	require.NoError(t, err)
	assert.False(t, defaults.Diff)
}

// TestCmd_TelemetryRecordsTheDiffFlag: adoption of the flag has to be
// readable without guessing at it from dry_run, so it is reported under its
// own key, reflecting the flag as given.
func TestCmd_TelemetryRecordsTheDiffFlag(t *testing.T) {
	withFlag := Cmd()
	require.NoError(t, withFlag.ParseFlags([]string{"--diff"}))

	event, ok := telemetry.EventFor(withFlag, nil)
	require.True(t, ok, "EventFor must return ok=true for an annotated command")

	assert.Equal(t, true, event.EventProperties["diff"])
	assert.Equal(t, false, event.EventProperties["dry_run"], "the pre-existing keys keep their own values")

	event, ok = telemetry.EventFor(Cmd(), nil)
	require.True(t, ok)
	assert.Equal(t, false, event.EventProperties["diff"], "an unset flag reports itself as off, not as absent")
}

// diffedResult is a result whose plan carries one change in each half, a
// whole row structure and an unmanaged path, the shape the JSON diff
// envelope tests read through the stubbed deploy.
func diffedResult() up.Result {
	return up.Result{
		Plan: up.Plan{
			State:    up.StateRunning,
			Code:     up.CodeChange{},
			Artifact: []up.Change{{Path: "containerGroups[default].containers[primary].port", Have: 8080.0, Want: 9090.0}},
			Runtime:  []up.Change{{Path: "containerGroups[default].replicaCount", Have: 1.0, Want: 3.0}},

			DiffArtifact: []up.DiffRow{{
				Path: "containerGroups[default].containers[primary].port",
				Have: 8080.0, Want: 9090.0, Changed: true,
			}},
			DiffRuntime: []up.DiffRow{{
				Path: "containerGroups[default].replicaCount",
				Have: 1.0, Want: 3.0, Changed: true,
			}},

			Unmanaged: []string{"containerGroups[default].containers[metrics]"},
		},
	}
}

// TestCmd_JSONDiffSectionFollowsTheFlag threads the flag into the envelope
// itself: with it, stdout is still exactly one JSON document and the plan
// inside it gains the structured diff section; without it, the plan has no
// diff key at all, which is what keeps a consumer that never asked for a diff
// from having to learn about one.
func TestCmd_JSONDiffSectionFollowsTheFlag(t *testing.T) {
	stubRun(t, diffedResult(), nil)

	stdout, _, err := runCmd(t, "--dry-run", "--output-format", "json", "--diff")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope),
		"stdout must be one JSON document and nothing else")

	body := envelope["up"].(map[string]any)

	plan, ok := body["plan"].(map[string]any)
	require.True(t, ok)

	diff, ok := plan["diff"].(map[string]any)
	require.True(t, ok, "--diff was requested, so the section is present")

	changes := diff["changes"].([]any)
	require.Len(t, changes, 2)

	first := changes[0].(map[string]any)

	assert.Equal(t, "containerGroups[default].containers[primary].port", first["path"])
	assert.InDelta(t, 8080.0, first["have"], 0)
	assert.InDelta(t, 9090.0, first["want"], 0)
	assert.Equal(t, false, first["absent"])

	unmanaged := diff["unmanaged"].([]any)
	require.Len(t, unmanaged, 1)
	assert.Equal(t, "containerGroups[default].containers[metrics]", unmanaged[0])

	// The legacy arrays keep their place beside the new section.
	artifact := plan["artifact"].([]any)
	require.Len(t, artifact, 1)
	assert.Equal(t, "containerGroups[default].containers[primary].port: 8080 -> 9090", artifact[0])

	stubRun(t, diffedResult(), nil)

	stdout, _, err = runCmd(t, "--dry-run", "--output-format", "json")
	require.NoError(t, err)

	envelope = map[string]any{}

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope),
		"stdout must be one JSON document and nothing else")

	body = envelope["up"].(map[string]any)
	plan = body["plan"].(map[string]any)

	assert.NotContains(t, plan, "diff", "without the flag the section is absent, not null")
}

// TestCmd_ConfirmIsRegistered: the flag exists, is opt-in, and its help says
// the one thing a reader could not guess -- that under non-interactive
// conditions it is suppressed rather than refused, because a flag that fails
// a scripted run helps nobody.
func TestCmd_ConfirmIsRegistered(t *testing.T) {
	lookup := Cmd().Flags().Lookup("confirm")

	require.NotNil(t, lookup, "the flag has to exist for --confirm to parse at all")
	assert.False(t, lookup.Hidden)
	assert.Equal(t, "false", lookup.DefValue, "confirming is opt-in")

	for _, term := range []string{
		"(y/N)", "--yes", "--output-format", "DATAROBOT_CLI_NON_INTERACTIVE",
		"terminal", "suppressed",
	} {
		assert.Contains(t, lookup.Usage, term, "the suppression matrix is documented in the flag itself")
	}
}

// TestCmd_ConfirmReachesTheDeploy threads the flag into the deploy's options,
// and only when it was given: a run without it must not start asking.
func TestCmd_ConfirmReachesTheDeploy(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmdWithInput(t, "y\n", "--confirm")
	require.NoError(t, err)
	require.NotNil(t, seen.ConfirmApply, "the gate reaches the deploy wired up")

	agreed, err := seen.ConfirmApply("Apply this deploy?")
	require.NoError(t, err)
	assert.True(t, agreed, "the answer is read from stdin")

	defaults := stubRun(t, deployed(), nil)

	_, _, err = runCmd(t)
	require.NoError(t, err)
	assert.Nil(t, defaults.ConfirmApply, "without the flag there is no gate")
}

// TestCmd_ConfirmGate_AnswersOnlyYes: the question is a y/N with the default
// on the safe side. Every spelling of yes proceeds; bare Enter, any other
// word, and a stdin that ends before an answer all decline.
func TestCmd_ConfirmGate_AnswersOnlyYes(t *testing.T) {
	cases := []struct {
		typed string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"Yes\n", true},
		{"yEs\n", true},
		{" y \n", true},
		{"\n", false},
		{"", false},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"nope\n", false},
		{"deploy\n", false},
	}

	for _, c := range cases {
		t.Run(c.typed, func(t *testing.T) {
			cmd := Cmd()

			var errOut bytes.Buffer

			cmd.SetErr(&errOut)
			cmd.SetIn(strings.NewReader(c.typed))

			ask := applyConfirm(cmd, true, false, bufio.NewReader(cmd.InOrStdin()))
			require.NotNil(t, ask)

			agreed, err := ask("Apply this deploy?")
			require.NoError(t, err)

			assert.Equal(t, c.want, agreed)
			assert.Contains(t, errOut.String(), "? Apply this deploy? (y/N)",
				"the question goes to stderr with the default shown")
		})
	}
}

// TestCmd_ConfirmGate_NotInstalledWhenSuppressed: the builder itself is nil
// whenever there is nobody to ask, which is the suppression-over-error
// convention the roll confirm follows.
func TestCmd_ConfirmGate_NotInstalledWhenSuppressed(t *testing.T) {
	cmd := Cmd()
	stdin := bufio.NewReader(cmd.InOrStdin())

	assert.Nil(t, applyConfirm(cmd, false, false, stdin), "no flag, no gate")
	assert.Nil(t, applyConfirm(cmd, true, true, stdin), "non-interactive swallows the flag")
	assert.Nil(t, applyConfirm(cmd, false, true, stdin), "and neither installs the other")
}

// --yes is already the answer, so --confirm alongside it behaves as if the
// flag was not given: no question, the same deploy.
func TestCmd_ConfirmSuppressed_Yes(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	stdout, stderr, err := runCmd(t, "--yes", "--confirm")
	require.NoError(t, err)

	assert.Nil(t, seen.ConfirmApply, "--yes is the answer, so there is no gate to install")
	assert.NotContains(t, stderr, "? Apply")
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout,
		"the suppressed run is byte-identical to one without --confirm")
}

// The non-interactive environment variable says nobody is reading, and a
// question nobody reads would hang a pipeline forever.
func TestCmd_ConfirmSuppressed_NonInteractiveEnv(t *testing.T) {
	onATerminal(t)
	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "1")

	seen := stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t, "--confirm")
	require.NoError(t, err)

	assert.Nil(t, seen.ConfirmApply)
	assert.NotContains(t, stderr, "? Apply")
}

// -o json implies non-interactive for the y/N gate specifically. The typed
// production confirm is a different question with different rules and is
// covered by TestCmd_JSONOutputStillHandsOverTheQuestion.
func TestCmd_ConfirmSuppressed_JSON(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	stdout, stderr, err := runCmd(t, "--output-format", "json", "--confirm")
	require.NoError(t, err)

	assert.Nil(t, seen.ConfirmApply)
	assert.True(t, json.Valid([]byte(stdout)), "stdout stays one JSON document")
	assert.NotContains(t, stderr, "Apply this deploy", "the y/N gate is the prompt that is suppressed")
}

// A piped stdin has nobody behind it, which is the CI case even with no other
// non-interactive signal set.
func TestCmd_ConfirmSuppressed_PipedStdin(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t, "--confirm")
	require.NoError(t, err)

	assert.True(t, seen.NonInteractive)
	assert.Nil(t, seen.ConfirmApply, "a piped stdin has nobody to ask")
	assert.NotContains(t, stderr, "? Apply")
}

// TestCmd_ConfirmSuppressed_RedirectedStderr: the question goes to stderr,
// so a terminal stdin with stderr sent to a file is a prompt nobody sees.
// The gate is suppressed the way the .env question decides it cannot ask,
// rather than left waiting on an answer that never comes.
func TestCmd_ConfirmSuppressed_RedirectedStderr(t *testing.T) {
	onATerminal(t)

	canAskFn = func(*cobra.Command) bool { return false }

	seen := stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t, "--confirm")
	require.NoError(t, err)

	assert.Nil(t, seen.ConfirmApply, "stderr is not a terminal, so there is nobody to ask")
	assert.NotContains(t, stderr, "? Apply")
}

// TestCmd_ConfirmQuestionGoesToStderr: with a terminal on both ends the
// question lands on stderr, and stdout stays the endpoint channel.
func TestCmd_ConfirmQuestionGoesToStderr(t *testing.T) {
	onATerminal(t)
	decliningRun(t, deployed())

	stdout, stderr, err := runCmdWithInput(t, "y\n", "--confirm")
	require.NoError(t, err)

	assert.Contains(t, stderr, "? Apply this deploy? (y/N)", "the question is on stderr")
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout,
		"stdout stays the endpoint channel")
}

// decliningRun wires the deploy the way the real one answers the gate, so the
// prompt, the parser and the command's decline mapping are exercised as one
// flow rather than as three unit-tested pieces.
//
// declined is what a declined run hands back: the real deploy returns the
// live workload it looked at, endpoint included, which is exactly what the
// shell must not print.
func decliningRun(t *testing.T, declined up.Result) {
	t.Helper()

	prev := runFn

	runFn = func(_ context.Context, opts up.Options) (up.Result, error) {
		if opts.ConfirmApply == nil {
			return deployed(), nil
		}

		ok, err := opts.ConfirmApply("Apply this deploy?")
		if err != nil {
			return up.Result{}, err
		}

		if !ok {
			declined.Action = up.ActionUnchanged

			return declined, up.ErrDeclined
		}

		return deployed(), nil
	}

	t.Cleanup(func() { runFn = prev })
}

// A decline after --sync-env re-sent a secret deploys nothing, but the
// rotation is in the store all the same, and the warning that the workload
// still serves the old value has to reach stderr on that run too.
func TestCmd_ConfirmDecline_StillWarnsAboutARotatedSecret(t *testing.T) {
	onATerminal(t)

	declined := deployed()
	declined.Env = up.EnvEdit{SecretsRotated: 1}

	decliningRun(t, declined)

	stdout, stderr, err := runCmdWithInput(t, "n\n", "--confirm", "--sync-env")
	require.ErrorIs(t, err, up.ErrDeclined)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "still serves the value it started with")
}

// TestCmd_ConfirmDecline_NonzeroExit: a decline is a refusal, and the command
// says so with a nonzero exit and one short line on stderr.
func TestCmd_ConfirmDecline_NonzeroExit(t *testing.T) {
	onATerminal(t)
	decliningRun(t, deployed())

	for _, input := range []string{"\n", "n\n", "nope\n", ""} {
		stdout, stderr, err := runCmdWithInput(t, input, "--confirm")

		require.Error(t, err, "answering %q must exit nonzero", input)
		require.ErrorIs(t, err, up.ErrDeclined)
		assert.Empty(t, stdout, "a declined run prints nothing on stdout")
		assert.Contains(t, stderr, "declined", "the decline is said on stderr")
		assert.Contains(t, stderr, "nothing was deployed")
		assert.NotContains(t, stderr, "https://app.datarobot.com/workloads/68b0/",
			"the endpoint of a workload the run left alone is not printed")
	}
}

// TestCmd_ConfirmDecline_StdoutEmpty is the stdout half on its own, because
// it is the half a script breaks on: whatever the decline's exit code, stdout
// must be byte-for-byte empty.
func TestCmd_ConfirmDecline_StdoutEmpty(t *testing.T) {
	onATerminal(t)
	decliningRun(t, deployed())

	stdout, _, err := runCmdWithInput(t, "n\n", "--confirm")
	require.Error(t, err)
	assert.Empty(t, stdout)
}

// TestCmd_ConfirmAccept_ProceedsToEndOfRun: accepting the y/N runs the normal
// success path, which in the shell means the endpoint lands on stdout as it
// always does.
func TestCmd_ConfirmAccept_ProceedsToEndOfRun(t *testing.T) {
	onATerminal(t)
	decliningRun(t, deployed())

	stdout, stderr, err := runCmdWithInput(t, "y\n", "--confirm")
	require.NoError(t, err)
	assert.Contains(t, stderr, "? Apply this deploy? (y/N)")
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout)
}

// TestCmd_ConfirmHelpDocumentsSuppression: the Long paragraph and the flag
// help between them tell a first-time reader what --diff prints, what
// --confirm asks, and when the asking is skipped.
func TestCmd_ConfirmHelpDocumentsSuppression(t *testing.T) {
	long := Cmd().Long

	assert.Contains(t, long, "--diff")
	assert.Contains(t, long, "--confirm")
	assert.Contains(t, long, "--dry-run --diff", "the look-without-touching combination is named")
	assert.Contains(t, long, "Only fields the manifest mentions are managed",
		"the pre-existing narrative stays")

	lookup := Cmd().Flags().Lookup("confirm")
	require.NotNil(t, lookup)
	assert.Contains(t, lookup.Usage, "behaves as if", "the suppressed semantics are stated, not implied")
}

// TestCmd_ConfirmAndDiffCompose: the two flags are independent, and cobra is
// deliberately not taught to refuse the pair -- suppression composes, mutual
// exclusivity does not.
func TestCmd_ConfirmAndDiffCompose(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--diff", "--confirm", "--dry-run")
	require.NoError(t, err, "the flags compose; refusing the pair would break a scripted caller")
	assert.True(t, seen.Diff)
	assert.True(t, seen.DryRun)
	assert.NotNil(t, seen.ConfirmApply)

	for _, name := range []string{"diff", "confirm"} {
		lookup := Cmd().Flags().Lookup(name)
		require.NotNil(t, lookup)

		assert.NotContains(t, lookup.Annotations, "cobra_annotation_mutually_exclusive",
			"--diff and --confirm are never made mutually exclusive")
	}
}

// TestCmd_TelemetryRecordsTheConfirmFlag: adoption of the flag has to be
// readable under its own key, reflecting the flag as given rather than the
// suppressed effective behavior.
func TestCmd_TelemetryRecordsTheConfirmFlag(t *testing.T) {
	withFlag := Cmd()
	require.NoError(t, withFlag.ParseFlags([]string{"--confirm"}))

	event, ok := telemetry.EventFor(withFlag, nil)
	require.True(t, ok, "EventFor must return ok=true for an annotated command")

	assert.Equal(t, true, event.EventProperties["confirm"])
	assert.Equal(t, false, event.EventProperties["diff"], "the pre-existing keys keep their own values")

	event, ok = telemetry.EventFor(Cmd(), nil)
	require.True(t, ok)
	assert.Equal(t, false, event.EventProperties["confirm"], "an unset flag reports itself as off, not as absent")
}
