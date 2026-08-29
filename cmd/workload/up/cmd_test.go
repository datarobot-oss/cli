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
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/up"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRun replaces the deploy and hands back what the caller asked for,
// recording the options the shell built.
func stubRun(t *testing.T, result up.Result, err error) *up.Options {
	t.Helper()

	seen := &up.Options{}
	prev := runFn

	runFn = func(opts up.Options) (up.Result, error) {
		*seen = opts

		return result, err
	}

	t.Cleanup(func() { runFn = prev })

	return seen
}

// runCmd executes the command with stdout and stderr captured separately,
// which is the only way to hold the JSON purity rule honestly.
func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	return runCmdWithInput(t, "", args...)
}

// runCmdWithInput is runCmd with something waiting on stdin, for the one
// question the deploy asks.
func runCmdWithInput(t *testing.T, input string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := Cmd()

	var out, errOut bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	// The command's own PreRunE authenticates, which a unit test must not do.
	cmd.PreRunE = nil

	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func deployed() up.Result {
	return up.Result{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Name:       "my-app",
		Status:     "running",
		Endpoint:   "https://app.datarobot.com/workloads/68b0/",
		ArtifactID: "68a0000000000000000000a1",
		Action:     up.ActionCreated,
	}
}

// TestCmd_StdoutCarriesOnlyTheEndpoint keeps `dr workload up | xargs curl`
// working: the plan and every progress line belong on stderr.
func TestCmd_StdoutCarriesOnlyTheEndpoint(t *testing.T) {
	stubRun(t, deployed(), nil)

	stdout, _, err := runCmd(t)
	require.NoError(t, err)
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout)
}

// TestCmd_JSONEnvelopeIsTheWholeOfStdout is the purity rule. Anything else on
// the stream breaks whoever is parsing it.
func TestCmd_JSONEnvelopeIsTheWholeOfStdout(t *testing.T) {
	stubRun(t, deployed(), nil)

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout must be one JSON document and nothing else")

	body, ok := envelope["up"].(map[string]any)
	require.True(t, ok, "the envelope is keyed on the command name")

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", body["workloadId"])
	assert.Equal(t, "my-app", body["name"])
	assert.Equal(t, "running", body["status"])
	assert.Equal(t, "created", body["action"])
	assert.Equal(t, false, body["locked"])
	assert.Contains(t, body, "plan", "a dry run in JSON is useless without the plan")
}

// TestCmd_BuildIDIsNullNotEmpty: a consumer has to be able to tell "no build
// happened" from "a build happened and produced nothing".
func TestCmd_BuildIDIsNullNotEmpty(t *testing.T) {
	stubRun(t, deployed(), nil)

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))

	body, _ := envelope["up"].(map[string]any)

	value, present := body["buildId"]
	assert.True(t, present, "the key has to be there so a consumer can read it")
	assert.Nil(t, value, "no build happened, which is not the same as a build that produced nothing")
}

// The other half of the same promise: a deploy that did build says which one,
// so a pipeline can fetch the logs without going looking for it.
func TestCmd_BuildIDIsCarriedWhenABuildRan(t *testing.T) {
	result := deployed()
	result.BuildID = "68c0000000000000000000b2"

	stubRun(t, result, nil)

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))

	body, _ := envelope["up"].(map[string]any)
	assert.Equal(t, "68c0000000000000000000b2", body["buildId"])
}

// TestCmd_JSONImpliesNonInteractive stops a wizard being drawn on stderr
// while stdout is being parsed.
func TestCmd_JSONImpliesNonInteractive(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)
	assert.True(t, seen.NonInteractive)
	assert.False(t, seen.Spinner, "nothing may draw over a machine-readable stream")
}

func TestCmd_YesImpliesNonInteractive(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--yes")
	require.NoError(t, err)
	assert.True(t, seen.NonInteractive)
}

// TestCmd_RefusesBindingFlags points at where binding actually lives instead
// of letting cobra say "unknown flag", which leaves the user guessing.
func TestCmd_RefusesBindingFlags(t *testing.T) {
	for _, flag := range []string{"--workload-id", "--name"} {
		t.Run(flag, func(t *testing.T) {
			stubRun(t, deployed(), nil)

			_, _, err := runCmd(t, flag, "whatever")
			require.Error(t, err)
			assert.Contains(t, err.Error(), ".datarobot.yaml")
			assert.Contains(t, err.Error(), "dr workload config")
		})
	}
}

// A --dir the OS would not call a directory is refused by name, the same as
// `dr workload delete` refuses it. Left to the manifest search it came back as
// a bare "not a directory: <abs path>" with nothing saying which flag put that
// path there, and the two commands print this flag at each other.
func TestCmd_RefusesADirThatIsNotADirectory(t *testing.T) {
	for name, dir := range map[string]string{
		"absent": "no-such-directory",
		"a file": filepath.Join(t.TempDir(), "file"),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "a file" {
				require.NoError(t, os.WriteFile(dir, []byte("x"), 0o600))
			}

			stubRun(t, deployed(), nil)

			_, _, err := runCmd(t, "--dir", dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--dir "+dir)
			assert.ErrorIs(t, err, manifest.ErrNotADirectory)
		})
	}
}

func TestCmd_DryRunSaysSoAndPrintsNoEndpoint(t *testing.T) {
	result := deployed()
	result.Endpoint = ""

	seen := stubRun(t, result, nil)

	stdout, stderr, err := runCmd(t, "--dry-run")
	require.NoError(t, err)
	assert.True(t, seen.DryRun)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "nothing was changed")
}

func TestCmd_PassesTheFlagsThrough(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	// A real directory, because --dir is now checked before the deploy runs.
	project := t.TempDir()

	_, _, err := runCmd(t, "--detach", "--dir", project)
	require.NoError(t, err)
	assert.True(t, seen.Detach)
	assert.Equal(t, project, seen.Dir)

	locking := stubRun(t, deployed(), nil)

	_, _, err = runCmd(t, "--lock")
	require.NoError(t, err)
	assert.True(t, locking.Lock)
}

// Locking happens after the workload is serving and --detach returns before
// that, so honouring both would drop the lock without saying so and leave the
// caller believing an artifact is permanent when it is still deletable.
func TestCmd_DetachAndLockCannotBeCombined(t *testing.T) {
	stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--detach", "--lock")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--lock cannot be combined with --detach")
}

// TestCmd_PollDefaultsApplyWithoutTheFlag guards the prototype's bug, where a
// flag that was never parsed left the wait at zero.
func TestCmd_PollDefaultsApplyWithoutTheFlag(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t)
	require.NoError(t, err)
	assert.Equal(t, defaultPollInterval, seen.PollInterval)
	assert.Equal(t, defaultPollTimeout, seen.PollTimeout)
}

func TestCmd_PollFlagsOverrideTheDefaults(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--poll-interval", "1s", "--poll-timeout", "2m")
	require.NoError(t, err)
	assert.Equal(t, "1s", seen.PollInterval.String())
	assert.Equal(t, "2m0s", seen.PollTimeout.String())
}

// TestCmd_FailureAfterTheWorkloadExistsStillEmitsTheEnvelope: losing the id
// because a later phase failed is how a deploy becomes unfindable.
func TestCmd_FailureAfterTheWorkloadExistsStillEmitsTheEnvelope(t *testing.T) {
	stubRun(t, deployed(), errors.New("timed out waiting"))

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.Error(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope),
		"the envelope has to be emitted before the failure surfaces")

	body, _ := envelope["up"].(map[string]any)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", body["workloadId"])
}

// A build that fails never creates a workload, so gating the envelope on the
// workload id alone would drop the one id the failure produced and leave a
// pipeline with nothing to fetch the logs with.
func TestCmd_FailedBuildStillEmitsTheEnvelope(t *testing.T) {
	stubRun(t, up.Result{BuildID: "68c0000000000000000000b2", Action: "created"},
		errors.New("build 68c0000000000000000000b2 finished as FAILED"))

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.Error(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope),
		"a failed build is still a build someone has to go and read")

	body, _ := envelope["up"].(map[string]any)
	assert.Equal(t, "68c0000000000000000000b2", body["buildId"])
	assert.Empty(t, body["workloadId"])
}

// TestCmd_FailureBeforeAnythingExistsJustFails: there is no id to report, so
// an envelope of empty strings would be noise.
func TestCmd_FailureBeforeAnythingExistsJustFails(t *testing.T) {
	stubRun(t, up.Result{}, errors.New("no .datarobot.yaml manifest"))

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, err.Error(), "manifest")
}

// The deploy asks one question, and only the exact name answers it. The
// question goes to stderr because stdout carries the endpoint.
func TestCmd_TypedConfirmAcceptsOnlyTheName(t *testing.T) {
	cases := []struct {
		typed string
		want  bool
	}{
		{"my-app\n", true},
		{"  my-app  \n", true},
		{"y\n", false},
		{"My-App\n", false},
		{"\n", false},
		{"", false},
	}

	for _, c := range cases {
		t.Run(c.typed, func(t *testing.T) {
			cmd := Cmd()

			var errOut bytes.Buffer

			cmd.SetErr(&errOut)
			cmd.SetIn(strings.NewReader(c.typed))

			agreed, err := typedConfirm(cmd)("Type the workload name: ", "my-app")
			require.NoError(t, err)

			assert.Equal(t, c.want, agreed)
			assert.Contains(t, errOut.String(), "Type the workload name: ")
		})
	}
}

// The question reaches the deploy wired up, so a locked production roll can
// actually be answered from a terminal.
func TestCmd_ConfirmIsHandedToTheDeploy(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmdWithInput(t, "my-app\n")
	require.NoError(t, err)
	require.NotNil(t, seen.Confirm)

	agreed, err := seen.Confirm("anything? ", "my-app")
	require.NoError(t, err)
	assert.True(t, agreed)
}

// onATerminal makes the run look like one a person is watching. A test
// harness always pipes stdin, so without this every test is the CI case.
func onATerminal(t *testing.T) {
	t.Helper()

	prev := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return true }

	t.Cleanup(func() { isStdinTerminalFn = prev })
}

// --yes is the answer, so there is nothing left to ask and the deploy is
// handed no way to ask it.
func TestCmd_YesHandsTheDeployNoQuestion(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--yes")
	require.NoError(t, err)

	assert.True(t, seen.NonInteractive)
	assert.Nil(t, seen.Confirm, "--yes already said yes")
}

// --output-format json keeps the wizard off the terminal, but it says how to
// format stdout rather than that production may be rolled without a word.
// Somebody is still there, so the question still reaches the deploy.
func TestCmd_JSONOutputStillHandsOverTheQuestion(t *testing.T) {
	onATerminal(t)

	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	assert.True(t, seen.NonInteractive, "no wizard may be drawn into the document")
	assert.NotNil(t, seen.Confirm, "but there is still someone to ask about production")
}

// With no terminal there is nobody to ask, which is the CI case: the deploy
// gets no question and rolls on the review the pipeline already had.
func TestCmd_NoTerminalHandsTheDeployNoQuestion(t *testing.T) {
	seen := stubRun(t, deployed(), nil)

	_, _, err := runCmd(t)
	require.NoError(t, err)

	assert.True(t, seen.NonInteractive)
	assert.Nil(t, seen.Confirm)
}

// The warning leads with one of these, and which one is not cosmetic. A dry
// run has changed nothing and may be previewing a workload that does not exist
// yet, so the present tense would contradict the "nothing was changed" line
// printed directly above it.
const (
	draftHeadline     = "This workload is running a draft artifact."
	draftHeadlinePlan = "This workload would run a draft artifact."
)

// draftWarned reports whether either tense reached the stream, for the tests
// that assert silence and have no reason to care which one was suppressed.
func draftWarned(stderr string) bool {
	return strings.Contains(stderr, draftHeadline) || strings.Contains(stderr, draftHeadlinePlan)
}

// A deploy onto a draft is on a clock nothing in the output used to mention:
// the workload is stopped eight hours after it starts running, whatever it is
// serving, so an endpoint shared in the afternoon can be dead by morning.
func TestCmd_DraftDeploySaysItIsTemporary(t *testing.T) {
	stubRun(t, deployed(), nil)

	stdout, stderr, err := runCmd(t)
	require.NoError(t, err)

	assert.Contains(t, stderr, draftHeadline)
	assert.Contains(t, stderr, "stopped after 8 hours")
	assert.Contains(t, stderr, "dr workload up --lock")

	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout,
		"the warning is prose and belongs on stderr, so the endpoint stays pipeable")
}

// The deadline is stated, and stated as a running time rather than an idle
// one. The platform's sweep selects on how long the workload has been running
// and never on whether it served anything, so "idle" or "inactive" here would
// be a comfortable lie to the one person who most needs the truth: someone
// about to share an endpoint they intend to keep using.
func TestCmd_DraftWarningStatesTheDeadline(t *testing.T) {
	stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)

	assert.Contains(t, stderr, "8 hours", "the figure is the platform's own published contract")
	assert.Contains(t, stderr, "whether or not they are in use", "the clock does not care about traffic")

	for _, wrong := range []string{"idle", "inactiv", "reclaim"} {
		assert.NotContains(t, stderr, wrong,
			"nothing deletes the artifact, and the timeout is not an inactivity one")
	}
}

// The trade the ticket asked for. Someone who wants to stop a workload goes
// looking for the command; someone whose workload stops by itself does not
// know there is anything to look for.
func TestCmd_DraftDeployTradesStopForLock(t *testing.T) {
	stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)

	assert.Contains(t, stderr, "dr workload up --lock")
	assert.NotContains(t, stderr, "dr workload stop")
	assert.Contains(t, stderr, "dr workload logs", "the other two lines stay")
	assert.Contains(t, stderr, "dr workload status")
}

// A locked artifact is permanent, so there is nothing to warn about and the
// stop line keeps its slot.
func TestCmd_LockedDeploySaysNothingExtra(t *testing.T) {
	result := deployed()
	result.Locked = true

	stubRun(t, result, nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)

	assert.False(t, draftWarned(stderr))
	assert.NotContains(t, stderr, "dr workload up --lock")
	assert.Contains(t, stderr, "dr workload stop")
}

// A --lock run is the remedy, so telling it about drafts would be telling it
// what it just did.
func TestCmd_LockFlagSaysNothingAboutDrafts(t *testing.T) {
	stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t, "--lock")
	require.NoError(t, err)
	assert.False(t, draftWarned(stderr))
}

// Someone previewing a first deploy is exactly who the warning is for, and a
// first deploy has no workload id yet, so the dry run cannot be gated on one.
func TestCmd_DryRunWarnsAboutADraftItWouldDeploy(t *testing.T) {
	stubRun(t, up.Result{Action: up.ActionCreated}, nil)

	stdout, stderr, err := runCmd(t, "--dry-run")
	require.NoError(t, err)

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "nothing was changed")
	assert.Contains(t, stderr, draftHeadlinePlan,
		"a preview has deployed nothing, so it may not claim the workload is already running one")
	assert.NotContains(t, stderr, draftHeadline)
	assert.Contains(t, stderr, "dr workload up --lock",
		"a dry run never reaches the Next list, so the warning has to carry the remedy itself")
}

func TestCmd_DryRunWithLockDoesNotWarn(t *testing.T) {
	stubRun(t, up.Result{Action: up.ActionCreated}, nil)

	_, stderr, err := runCmd(t, "--dry-run", "--lock")
	require.NoError(t, err)
	assert.False(t, draftWarned(stderr))
}

// The purity rule, held against the one thing added since it was written: the
// warning is prose, and prose never reaches a machine-readable stream.
func TestCmd_JSONOutputCarriesNoDraftProse(t *testing.T) {
	stubRun(t, deployed(), nil)

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "stdout must stay one JSON document")
	assert.False(t, draftWarned(stdout))

	body, _ := envelope["up"].(map[string]any)
	assert.Equal(t, false, body["locked"], "the envelope already says this in a form a machine can read")
}

// Nothing was deployed, so there is no endpoint with a clock on it and the
// warning would be about nothing. Same guard the Next list already uses.
func TestCmd_NoWorkloadSaysNothingAboutDrafts(t *testing.T) {
	stubRun(t, up.Result{BuildID: "68c0000000000000000000b2"}, errors.New("build failed"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)
	assert.False(t, draftWarned(stderr))
}

// A failed deploy has one thing worth reading and it is the error. Telling
// someone their broken deploy is also temporary buries it.
func TestCmd_FailedDeployDoesNotWarnAboutDrafts(t *testing.T) {
	stubRun(t, deployed(), errors.New("timed out waiting"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)
	assert.False(t, draftWarned(stderr))
}

// The exception, and the reason it exists: a deploy onto a stopped workload
// starts it before it rolls, so a run that fails after that has itself put a
// draft on the air and started the eight hours. Saying nothing leaves a
// workload running that the reader believes was never touched.
func TestCmd_FailedDeployThatStartedTheWorkloadStillWarns(t *testing.T) {
	result := deployed()
	result.Action = up.ActionStarted

	stubRun(t, result, errors.New("the rollout of workload wl-1 ended as failed"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)
	assert.True(t, draftWarned(stderr))
}

// A run that changed nothing still leaves a draft serving on the same clock.
// "Already up to date" is not the same as "this will still be here tomorrow".
func TestCmd_UnchangedRunStillWarnsAboutTheDraft(t *testing.T) {
	result := deployed()
	result.Action = up.ActionUnchanged

	stubRun(t, result, nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)
	assert.Contains(t, stderr, draftHeadline)
}

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

func TestCmd_IsRegisteredUnderWorkload(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "up", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("detach"))
	assert.NotNil(t, cmd.Flags().Lookup("lock"))
	assert.True(t, cmd.Flags().Lookup("poll-interval").Hidden)
}

// The commands are runnable as printed from the project that was just
// deployed, which is the whole point of dropping the id from them.
func TestCmd_NextStepsCarryNoID(t *testing.T) {
	stubRun(t, deployed(), nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)

	assert.Contains(t, stderr, "dr workload logs  ")
	assert.NotContains(t, stderr, "dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0")
}

// A deploy that ran elsewhere needs the flag that reaches it, because the
// manifest search only walks upward.
func TestCmd_NextStepsCarryDirWhenTheDeployDid(t *testing.T) {
	stubRun(t, deployed(), nil)

	dir := t.TempDir()

	_, stderr, err := runCmd(t, "--dir", dir)
	require.NoError(t, err)

	assert.Contains(t, stderr, "dr workload logs --dir "+dir)
	assert.Contains(t, stderr, "dr workload up --lock --dir "+dir,
		"every line in the block has to run as printed, --lock included")
}

// The one shape where bare commands would not resolve: a workload was created
// but its id could not be written back, so the manifest holds no binding. The
// id is named instead, because a deploy that failed late still has to be
// findable.
func TestCmd_FailedRunStillNamesTheWorkload(t *testing.T) {
	stubRun(t, deployed(), errors.New("id could not be written"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.Contains(t, stderr, "68b0c1d2e3f4a5b6c7d8e9f0")
}

// --lock takes no id, so on a failed run it cannot be made to name the
// workload, and that is the run whose manifest may hold no binding. Printed
// bare it would create a second workload instead of locking this one.
func TestCmd_FailedDraftRunOmitsTheLockLine(t *testing.T) {
	result := deployed()
	result.Action = up.ActionStarted

	stubRun(t, result, errors.New("id could not be written"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	_, next, found := strings.Cut(stderr, "Next:")
	require.True(t, found)

	// Scoped to the block: the draft warning above it names the same command
	// as prose, and says the same thing on the successful runs where it is
	// sound. This is about the copy-and-run list.
	assert.NotContains(t, next, "--lock")
	assert.Contains(t, next, "dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0",
		"the lines that can name the workload still do")
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

			ask := applyConfirm(cmd, true, false)
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

	assert.Nil(t, applyConfirm(cmd, false, false), "no flag, no gate")
	assert.Nil(t, applyConfirm(cmd, true, true), "non-interactive swallows the flag")
	assert.Nil(t, applyConfirm(cmd, false, true), "and neither installs the other")
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

// TestCmd_ConfirmInstallKeysOnStdinNotStderr: the gate follows stdin, not
// stderr. A run with a terminal stdin still asks when stderr is redirected,
// and the question lands in whatever the stderr is.
func TestCmd_ConfirmInstallKeysOnStdinNotStderr(t *testing.T) {
	onATerminal(t)
	decliningRun(t)

	stdout, stderr, err := runCmdWithInput(t, "y\n", "--confirm")
	require.NoError(t, err)

	assert.Contains(t, stderr, "? Apply this deploy? (y/N)", "the question is on stderr")
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout,
		"stdout stays the endpoint channel")
}

// decliningRun wires the deploy the way the real one answers the gate, so the
// prompt, the parser and the command's decline mapping are exercised as one
// flow rather than as three unit-tested pieces.
func decliningRun(t *testing.T) {
	t.Helper()

	prev := runFn

	runFn = func(opts up.Options) (up.Result, error) {
		ok, err := opts.ConfirmApply("Apply this deploy?")
		if err != nil {
			return up.Result{}, err
		}

		if !ok {
			// WorkloadID is set on purpose: a declined run of an existing
			// workload is exactly the case where falling through to the
			// renderer would leak the endpoint onto stdout.
			return up.Result{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}, up.ErrDeclined
		}

		return deployed(), nil
	}

	t.Cleanup(func() { runFn = prev })
}

// TestCmd_ConfirmDecline_NonzeroExit: a decline is a refusal, and the command
// says so with a nonzero exit and one short line on stderr.
func TestCmd_ConfirmDecline_NonzeroExit(t *testing.T) {
	onATerminal(t)
	decliningRun(t)

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
	decliningRun(t)

	stdout, _, err := runCmdWithInput(t, "n\n", "--confirm")
	require.Error(t, err)
	assert.Empty(t, stdout)
}

// TestCmd_ConfirmAccept_ProceedsToEndOfRun: accepting the y/N runs the normal
// success path, which in the shell means the endpoint lands on stdout as it
// always does.
func TestCmd_ConfirmAccept_ProceedsToEndOfRun(t *testing.T) {
	onATerminal(t)
	decliningRun(t)

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
