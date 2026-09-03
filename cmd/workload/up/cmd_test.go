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

	// The same run has no follow-ups either, and for its own reason: every
	// command in the list needs an id, and this run never produced one. Pinned
	// here because the guard that says so sits above the failed branch in
	// followUps, where a later edit could reorder it out of the way.
	assert.NotContains(t, stderr, "Next:")
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

// A refused --dry-run is a failure, and its footer would read as a preview
// that went to plan. The plan block above it already says it is disclaimed and
// the error below says why, so the line between them is the one thing on the
// screen still describing an ordinary run.
func TestCmd_FailedDryRunDropsTheFooter(t *testing.T) {
	stubRun(t, refused(up.StateTerminated), errors.New("workload my-app is terminated"))

	_, stderr, err := runCmd(t, "--dry-run")
	require.Error(t, err)

	assert.NotContains(t, stderr, "Dry run: nothing was changed.")
	assert.False(t, draftWarned(stderr))
}

// refused is the result a deploy hands back when the live state turned it
// away: the workload is real and reports where it stands, and nothing was done
// to it. Status and the plan's state agree here because a refusal sets the
// first from the second, which is the shape the shell reads.
func refused(state up.State) up.Result {
	result := deployed()
	result.Status = state.String()
	result.Plan.State = state
	result.Action = up.ActionUnchanged

	return result
}

// TestCmd_TerminatedRefusalOffersNoFollowUps is half the reported bug. Three
// commands were printed under the error, and not one of them applies to a
// workload that is not coming back: 'stop' on a terminated workload is
// meaningless, and the refusal already names the delete that clears the way.
func TestCmd_TerminatedRefusalOffersNoFollowUps(t *testing.T) {
	stubRun(t, refused(up.StateTerminated), errors.New("workload my-app is terminated"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.NotContains(t, stderr, "Next:")
	assert.NotContains(t, stderr, "dr workload stop")
	assert.NotContains(t, stderr, "dr workload logs")
}

// An errored workload is the other half, and it goes the other way: reading the
// logs is exactly what to do next, and the list is the only place the id is
// attached to the command. What it loses is 'stop', which is not a next step
// for a deploy that never landed.
func TestCmd_ErroredRefusalKeepsLogsAndStatusButNotStop(t *testing.T) {
	stubRun(t, refused(up.StateErrored), errors.New("workload my-app is errored"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.Contains(t, stderr, "Next:")
	assert.Contains(t, stderr, "dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.Contains(t, stderr, "dr workload status 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, stderr, "dr workload stop")
}

// The rule is about the run having failed rather than about a refusal: a wait
// that timed out, or a rollout that never completed, leaves a running workload
// whose logs and status are the whole of what there is to look at.
func TestCmd_AnyFailureKeepsLogsAndStatusButNotStop(t *testing.T) {
	stubRun(t, deployed(), errors.New("timed out waiting"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.Contains(t, stderr, "dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.Contains(t, stderr, "dr workload status 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, stderr, "dr workload stop")
	assert.NotContains(t, stderr, "dr workload up --lock",
		"a failed run is not advised to lock what it did not deploy")
}

// A deploy onto a stopped workload starts it before it rolls, so a run that
// fails after that has itself put a draft on the air: draftIsServing says so,
// and the warning prints. The list still does not offer --lock, because locking
// a version this run could not finish is not the remedy; the warning names the
// command inline for anyone who decides otherwise.
func TestCmd_FailureAfterAStartWarnsButOffersNoLock(t *testing.T) {
	result := deployed()
	result.Action = up.ActionStarted

	stubRun(t, result, errors.New("rollout never completed"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.True(t, draftWarned(stderr), "the run left a draft running and has to say so")
	assert.Contains(t, stderr, "dr workload logs 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.Contains(t, stderr, "dr workload status 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, stderr, "  dr workload up --lock  Lock the artifact")
	assert.NotContains(t, stderr, "dr workload stop")
}

// A run that started a workload which then reached the end of its life warns
// about nothing and offers nothing. The draft clock this would otherwise
// mention is not ticking, because a terminated workload is running no artifact
// at all, and the two halves of the footer have to agree: a warning above an
// empty follow-up list reads as advice the command forgot to finish.
func TestCmd_StartedThenTerminatedWarnsAboutNoDraft(t *testing.T) {
	result := deployed()
	result.Action = up.ActionStarted
	result.Status = "terminated"

	stubRun(t, result, errors.New("workload my-app finished as terminated"))

	_, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.False(t, draftWarned(stderr), "nothing is running, so nothing is on a clock")
	assert.NotContains(t, stderr, "Next:")
}

// A refusal still reports the workload it was refused by. Losing the id is how
// a deploy becomes unfindable, and the endpoint is deliberately kept for the
// same reason; only its success tick is dropped.
func TestCmd_TerminatedRefusalStillReportsTheEndpoint(t *testing.T) {
	stubRun(t, refused(up.StateTerminated), errors.New("workload my-app is terminated"))

	stdout, stderr, err := runCmd(t)
	require.Error(t, err)

	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/\n", stdout)
	assert.Contains(t, stderr, "Workload endpoint:")
	assert.NotContains(t, stderr, "✓ Workload endpoint:")
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
}

// rotatedNothingElse is the run --sync-env exists to make safe: the secret
// reached the credential store, the manifest was already current, and so no
// container was replaced on the way past.
func rotatedNothingElse() up.Result {
	result := deployed()
	result.Action = up.ActionUnchanged
	result.Env = up.EnvEdit{SecretsRotated: 1}

	return result
}

// A rotation the deploy could not finish is true of the workload whatever the
// output format. The warning used to sit below the JSON return, so the one run
// that most needs it, a scripted rotation, was the one that never saw it.
func TestCmd_JSONStillWarnsThatTheOldSecretIsServing(t *testing.T) {
	stubRun(t, rotatedNothingElse(), nil)

	stdout, stderr, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	assert.Contains(t, stderr, "still serves the value it started with")
	assert.Contains(t, stderr, "dr workload stop --yes")

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), "the advice goes to stderr, so stdout stays pure")
}

// The same run without --output-format json, so the two formats are known to
// agree rather than assumed to.
func TestCmd_TextWarnsThatTheOldSecretIsServing(t *testing.T) {
	stubRun(t, rotatedNothingElse(), nil)

	_, stderr, err := runCmd(t)
	require.NoError(t, err)
	assert.Contains(t, stderr, "still serves the value it started with")
}

// The envelope carries the names `dr workload config` reports under
// envLiterals, and carries them as a list: a consumer that iterates should not
// have to guard for null on the run that wrote nothing.
func TestCmd_EnvLiteralsAreAlwaysAList(t *testing.T) {
	result := deployed()
	result.Env = up.EnvEdit{KeysAdded: 1, Literals: []string{"REGION"}}

	stubRun(t, result, nil)

	stdout, _, err := runCmd(t, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))

	body, _ := envelope["up"].(map[string]any)
	env, ok := body["env"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"REGION"}, env["literals"])

	stubRun(t, deployed(), nil)

	stdout, _, err = runCmd(t, "--output-format", "json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))

	body, _ = envelope["up"].(map[string]any)
	env, _ = body["env"].(map[string]any)
	assert.Equal(t, []any{}, env["literals"], "a run with no .env flags names nothing, which is not null")
}
