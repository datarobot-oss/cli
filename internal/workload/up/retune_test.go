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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retuned is the live built project with one runtime number moved and nothing
// else, which is the whole of a settings change: same artifact, same code,
// more of it.
func retuned(from, to string) string {
	return "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest, from, to, 1)
}

// wiredRetune answers the settings call and the two waits that follow it: the
// replacement the platform starts to apply the sizing, then the workload.
//
// The guard is recorded rather than left to install's silent no-op, the way
// the roll fixture records it. Otherwise the sequence assertions below read as
// the whole of a resize while saying nothing about the one call that has to
// come before the PATCH, and deleting it would leave them green.
func wiredRetune(tr *track, sent *json.RawMessage) fakes {
	return fakes{
		workloadD: func(string) (workload.Document, error) { return docOf(liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return docOf(liveArtifactJSON), nil },
		guard: func(workloadID string) error {
			tr.steps = append(tr.steps, "guard:"+workloadID)

			return nil
		},
		settings: func(workloadID string, runtime json.RawMessage) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "settings:"+workloadID)
			*sent = runtime

			return &workload.Replacement{ID: "rep-1", WorkloadID: workloadID}, nil
		},
		waitReplace: func(_ string, started *workload.Replacement, _, _ time.Duration,
			_ func(*workload.Replacement),
		) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "await-resize")
			started.Status = workload.ReplacementStatusCompleted

			return started, nil
		},
		// A resize changes no version, so it must not tell the wait to expect
		// one: there is nothing for the workload to promote, and a wait given an
		// artifact that is never going to move would run to its timeout.
		wait: func(id, want string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			tr.steps = append(tr.steps, "settle:"+want)

			return &workload.Workload{
				ID: id, Name: "gpt-oss-20b-vllm", Status: workload.WorkloadStatusRunning,
				ArtifactID: "68a0000000000000000000a1", Endpoint: "https://app.datarobot.com/workloads/68b0/",
			}, nil
		},
	}
}

// A resize mutates once. Nothing is built, nothing is minted and nothing is
// swapped, which is the entire reason this path exists rather than being
// folded into a roll. The guard ahead of it is a read, and it is in the
// sequence because a resize is a replacement like any other.
func TestRun_RuntimeOnlyChangeIsAppliedInPlace(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("a sizing change mints no version")

		return nil, nil
	}
	f.replace = func(string, string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("a sizing change swaps nothing")

		return nil, nil
	}

	install(t, f)

	result, stderr, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{
			"guard:68b0c1d2e3f4a5b6c7d8e9f0", "settings:68b0c1d2e3f4a5b6c7d8e9f0",
			"await-resize", "settle:",
		}, tr.steps,
		"the platform resizes by rolling a replacement, so that is what has to be guarded and followed")
	assert.Equal(t, ActionUpdated, result.Action)
	assert.Equal(t, workload.WorkloadStatusRunning, result.Status, "the wait has the last word on status")
	assert.Equal(t, "68a0000000000000000000a1", result.ArtifactID, "the version serving does not change")
	assert.Contains(t, stderr, "~ runtime")
}

// The whole block goes, not the fields that differ: the platform takes it as
// a unit, and a subset would leave the reader guessing which half is live.
func TestRun_RetuneSendsTheWholeRuntimeBlock(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	install(t, wiredRetune(&tr, &sent))

	_, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.NoError(t, err)

	var block map[string]any

	require.NoError(t, json.Unmarshal(sent, &block))

	groups, ok := block["containerGroups"].([]any)
	require.True(t, ok, "the runtime block travels as the file wrote it")
	assert.Len(t, groups, 1)

	group, ok := groups[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "default", group["name"], "fields that did not change travel too")
}

// A credential this run does not touch must not be able to refuse a resize:
// the artifact keeps every variable it already has, and nothing about the
// environment is sent.
func TestRun_RetuneDoesNotVerifyCredentials(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.cred = func(string) (*workload.Credential, error) {
		t.Fatal("a settings update sends no environment")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Contains(t, tr.steps, "settings:68b0c1d2e3f4a5b6c7d8e9f0")
}

// A resize is a replacement like any other, so it has to ask the question a
// roll asks before the PATCH goes out.
func TestRun_RetuneRefusesWhileAReplacementIsInFlight(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.guard = func(workloadID string) error {
		tr.steps = append(tr.steps, "guard:"+workloadID)

		return fmt.Errorf("workload %s: %w (status switching); wait for it to settle before starting another",
			workloadID, workload.ErrReplacementInFlight)
	}
	f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("the sizing may not ride in behind a rollout that is still deciding")

		return nil, nil
	}

	install(t, f)

	result, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a replacement is already in progress")
	assert.NotContains(t, err.Error(), "cannot tell whether",
		"a refusal already names the workload and the remedy, so it travels in its own words")
	assert.Equal(t, []string{"guard:68b0c1d2e3f4a5b6c7d8e9f0"}, tr.steps,
		"the workload the file is bound to is the one that has to be clear")
	assert.Equal(t, ActionUnchanged, result.Action,
		"a run refused before it touched anything did not update anything")
}

// The guard has two failure modes and they read differently. A route that
// cannot answer says nothing about the resize it stopped, so the operation is
// named for it.
func TestRun_RetuneNamesTheResizeWhenTheGuardCannotAnswer(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.guard = func(string) error { return errors.New("HTTP error: 500 Internal Server Error") }
	f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("a guard that could not answer is not permission to proceed")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"cannot tell whether workload 68b0c1d2e3f4a5b6c7d8e9f0 already has a rollout in progress, "+
			"so its runtime settings were left alone")
	assert.Contains(t, err.Error(), "500 Internal Server Error", "the reason survives the wrapping")
}

func TestRun_RetuneFailureNamesTheWorkload(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
		return nil, errors.New("replicaCount and autoscaling are mutually exclusive")
	}
	f.wait = func(string, string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		t.Fatal("a settings call that failed changed nothing to wait for")

		return nil, nil
	}

	install(t, f)

	result, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "68b0c1d2e3f4a5b6c7d8e9f0")
	assert.Contains(t, err.Error(), "mutually exclusive", "the platform's reason has to survive")
	assert.Equal(t, ActionUpdated, result.Plan.Action(), "the plan still says what was asked for")
}

// The workload stays running for the whole swap, so its status can never say
// a resize failed. Only the replacement can, and a run that ignored it would
// report the old sizing as the new one.
func TestRun_RetuneReportsAReplacementThatFailed(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	f.waitReplace = func(_ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		started.Status = workload.ReplacementStatusFailed

		return started, errors.New("replacement ended as FAILED")
	}
	f.wait = func(string, string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		t.Fatal("a resize that failed leaves nothing to settle")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still running with the sizing it had")
	assert.Contains(t, err.Error(), "68b0c1d2e3f4a5b6c7d8e9f0")
}

func TestRun_DetachedRetuneDoesNotWait(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	install(t, wiredRetune(&tr, &sent))

	result, stderr, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)

	assert.NotContains(t, tr.steps, "await-resize")
	assert.NotContains(t, tr.steps, "settle:")
	assert.Equal(t, ActionUpdated, result.Action)
	// The status is the live one, because nobody looked again. A settings
	// change leaves the workload running while the replacement swaps under
	// it, so there is no other status to report without waiting.
	assert.Equal(t, workload.WorkloadStatusRunning, result.Status)
	assert.Contains(t, stderr, "not waiting")
}

// A file that moved the sizing and the version rides both in on one swap. Two
// operations would either resize the version being rolled off, or start the
// new one under the old sizing.
func TestRun_RollCarriesTheRuntimeWithTheSwap(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")
	f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("the swap carries the sizing; a second call would apply it twice")

		return nil, nil
	}

	install(t, f)

	both := strings.Replace(builtDrift(), "cpu: 3", "cpu: 6", 1)

	result, _, err := runIn(t, both, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, ActionRolled, result.Action, "a version change outranks a resize")
	require.NotNil(t, tr.rolledRuntime, "the sizing has to reach the platform somehow")

	var block map[string]any

	require.NoError(t, json.Unmarshal(tr.rolledRuntime, &block))
	assert.Contains(t, block, "containerGroups")
}

// A rollout that changes no sizing sends no sizing. Sending the block anyway
// would make every roll a settings change as well.
func TestRun_RollWithoutARuntimeChangeSendsNoRuntime(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Nil(t, tr.rolledRuntime)
}

// --lock is about the artifact that ends up live, not about one this run
// minted. A resize mints nothing, so the artifact already serving is the one
// that ends up live, and it is what gets locked. A start does the same.
func TestRun_RetuneWithLockLocksTheServingArtifact(t *testing.T) {
	var (
		tr   track
		sent json.RawMessage
	)

	f := wiredRetune(&tr, &sent)
	// The shared fixture is already locked, which is the other branch: an
	// artifact cannot be locked twice.
	f.artifactD = func(string) (workload.Document, error) { return unlockedArtifact(t), nil }
	f.lock = func(id string) (*workload.Artifact, error) {
		tr.steps = append(tr.steps, "lock:"+id)

		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
	}

	install(t, f)

	result, _, err := runIn(t, retuned("cpu: 3", "cpu: 6"), Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"guard:68b0c1d2e3f4a5b6c7d8e9f0", "settings:68b0c1d2e3f4a5b6c7d8e9f0",
		"await-resize", "settle:", "lock:68a0000000000000000000a1",
	}, tr.steps, "the lock comes last, once the workload is serving again")
	assert.True(t, result.Locked)
}
