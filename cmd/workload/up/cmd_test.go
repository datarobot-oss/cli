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
	"testing"

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

	cmd := Cmd()

	var out, errOut bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
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

	_, _, err := runCmd(t, "--detach", "--dir", "/tmp/project")
	require.NoError(t, err)
	assert.True(t, seen.Detach)
	assert.Equal(t, "/tmp/project", seen.Dir)

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

func TestCmd_IsRegisteredUnderWorkload(t *testing.T) {
	cmd := Cmd()

	assert.Equal(t, "up", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("detach"))
	assert.NotNil(t, cmd.Flags().Lookup("lock"))
	assert.True(t, cmd.Flags().Lookup("poll-interval").Hidden)
}
