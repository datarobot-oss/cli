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
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The live pair below says exactly what boundImageManifest says, so a run
// against it plans nothing. Every roll test moves one field and no more, and
// the field it moves is in the artifact, because that is what mints a
// version.
const (
	liveImageWorkloadJSON = `{
  "id": "68b0c1d2e3f4a5b6c7d8e9f0",
  "name": "my-app",
  "status": "running",
  "importance": "low",
  "artifactId": "68a0000000000000000000a1",
  "endpoint": "https://app.datarobot.com/workloads/68b0/",
  "runtime": {
    "containerGroups": [
      {
        "name": "default",
        "replicaCount": 1,
        "containers": [
          {"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}
        ]
      }
    ]
  }
}`

	liveImageArtifactJSON = `{
  "id": "68a0000000000000000000a1",
  "name": "my-app-artifact",
  "status": "draft",
  "spec": {
    "type": "service",
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {"name": "primary", "primary": true, "port": 8080, "imageUri": "registry/team/app:v1"}
        ]
      }
    ]
  }
}`
)

// boundImageManifest is unboundImageManifest tied to the live workload above.
const boundImageManifest = "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

// newImage is the same file asking for a different image, which is the
// smallest change that needs a new version rolled in.
func newImage() string {
	return strings.Replace(boundImageManifest, "app:v1", "app:v2", 1)
}

// liveImage points the two reads at the running pair.
func liveImage(f fakes) fakes {
	f.workloadD = func(string) (workload.Document, error) { return docOf(liveImageWorkloadJSON), nil }
	f.artifactD = func(string) (workload.Document, error) { return docOf(liveImageArtifactJSON), nil }

	return f
}

// docOf is doc without a *testing.T, so the fixtures above can be built
// inside the seams themselves, which take no test to call.
func docOf(raw string) workload.Document {
	var d workload.Document

	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		panic(err)
	}

	return d
}

// wiredRoll is a roll where every step succeeds, recording the order they
// happened in. Order is most of what this path has to get right.
func wiredRoll(tr *track) fakes {
	return liveImage(fakes{
		guard: func(string) error {
			tr.steps = append(tr.steps, "guard")

			return nil
		},
		newArtifact: func(payload any) (*workload.Artifact, error) {
			tr.steps = append(tr.steps, "create-artifact")
			tr.artifactIn = payload

			return &workload.Artifact{ID: "art-2", Status: workload.ArtifactStatusDraft}, nil
		},
		lock: func(id string) (*workload.Artifact, error) {
			tr.steps = append(tr.steps, "lock:"+id)

			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
		replace: func(workloadID, artifactID string) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "replace:"+artifactID)

			return &workload.Replacement{ID: "rep-1", WorkloadID: workloadID, ArtifactID: artifactID}, nil
		},
		waitReplace: func(_ string, started *workload.Replacement, _, _ time.Duration,
			_ func(*workload.Replacement),
		) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "await-rollout")
			started.Status = workload.ReplacementStatusCompleted

			return started, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			tr.steps = append(tr.steps, "settle")

			return &workload.Workload{
				ID: id, Name: "my-app", Status: workload.WorkloadStatusRunning,
				ArtifactID: "art-2", Endpoint: "https://app.datarobot.com/workloads/68b0/",
			}, nil
		},
	})
}

// locked makes the running version production.
func lockedLive(f fakes) fakes {
	f.artifactD = func(string) (workload.Document, error) {
		d := docOf(liveImageArtifactJSON)
		d["status"] = workload.ArtifactStatusLocked

		return d, nil
	}

	return f
}

func TestRun_LivePairWithNoDriftPlansNothing(t *testing.T) {
	install(t, liveImage(fakes{}))

	result, stderr, err := runIn(t, boundImageManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action, "the fixtures have to agree or every roll test is testing noise")
	assert.Contains(t, stderr, "Already up to date")
}

func TestRun_RollsALiveWorkloadOntoANewVersion(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	result, stderr, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"guard", "create-artifact", "guard", "replace:art-2", "await-rollout", "settle"}, tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
	assert.Equal(t, "art-2", result.ArtifactID)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", result.WorkloadID, "a roll never makes a second workload")
	assert.Equal(t, "https://app.datarobot.com/workloads/68b0/", result.Endpoint, "the endpoint survives a rollout")
	assert.False(t, result.Locked, "a draft rolls onto a draft")
	assert.Contains(t, stderr, "+ artifact")
}

// A file that names an artifact is deploying one that already exists, so
// creating another would leave a stray behind on every run.
func TestRun_RollUsesTheArtifactTheFileNames(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("the file named the version to roll onto")

		return nil, nil
	}

	install(t, f)

	named := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundArtifactManifest

	result, _, err := runIn(t, named, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"guard", "guard", "replace:68b0bbbb0000000000000002", "await-rollout", "settle"}, tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
}

// The up-front guard exists so a doomed run fails before it creates a version
// nobody will ever roll.
func TestRun_ReplacementInFlightStopsBeforeAnythingIsMade(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.guard = func(string) error { return errors.New("workload wl-1 already has a replacement in progress") }

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a replacement in progress")
	assert.Empty(t, tr.steps, "nothing may be created while a rollout is running")
}

// Decision 1: rolling production is allowed, and typing the name is the whole
// ceremony. The new version is locked before the swap, because the platform
// will not replace a locked version with a draft.
func TestRun_LockedProductionRollsAfterTheNameIsTyped(t *testing.T) {
	var (
		tr     track
		asked  string
		expect string
	)

	install(t, lockedLive(wiredRoll(&tr)))

	result, _, err := runIn(t, newImage(), Options{
		Confirm: func(question, want string) (bool, error) {
			asked, expect = question, want

			return true, nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"guard", "create-artifact", "guard", "lock:art-2", "replace:art-2", "await-rollout", "settle"},
		tr.steps, "the successor is locked after the last guard and before the swap")
	assert.Contains(t, asked, "production")
	assert.Equal(t, "my-app", expect, "the name is what has to be typed")
	assert.True(t, result.Locked)
}

// A plan with both halves is refused rather than half-applied. Rolling and
// leaving the sizing behind would carry out part of the plan just printed and
// report the whole of it as done, which is worse than not starting.
func TestRun_RollThatAlsoResizesIsRefusedWholesale(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	both := strings.Replace(newImage(), "cpu: 0.5", "cpu: 2", 1)

	_, _, err := runIn(t, both, Options{NonInteractive: true})
	require.ErrorIs(t, err, ErrNotWired)
	assert.Contains(t, err.Error(), "runtime settings")
	assert.Empty(t, tr.steps, "nothing may be rolled while half the plan cannot be applied")
}

// Locking cannot be undone and a locked artifact cannot be deleted, so a lock
// taken before the final guard is stranded when that guard refuses. The guard
// has to lose the race first.
func TestRun_LockIsNotTakenWhenTheFinalGuardRefuses(t *testing.T) {
	var (
		tr    track
		calls int
	)

	f := lockedLive(wiredRoll(&tr))

	// The first guard passes and the second finds a rollout someone else
	// started meanwhile, which is the race this ordering exists for.
	f.guard = func(string) error {
		tr.steps = append(tr.steps, "guard")
		calls++

		if calls == 1 {
			return nil
		}

		return errors.New("workload wl-1 already has a replacement in progress")
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{
		Confirm: func(string, string) (bool, error) { return true, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a replacement in progress")
	assert.NotContains(t, tr.steps, "lock:art-2",
		"a refused rollout must not leave an artifact locked for a swap that never happened")
}

func TestRun_LockedProductionLeftAloneWhenTheAnswerIsNo(t *testing.T) {
	var tr track

	install(t, lockedLive(wiredRoll(&tr)))

	_, _, err := runIn(t, newImage(), Options{
		Confirm: func(string, string) (bool, error) { return false, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still running the version it was")
	assert.NotContains(t, tr.steps, "lock:art-2", "locking cannot be undone, so it must not happen on a no")
	assert.NotContains(t, tr.steps, "replace:art-2")
}

// Being unable to ask is not the same as having been told to go ahead.
func TestRun_LockedProductionWithNoWayToAskRefuses(t *testing.T) {
	var tr track

	install(t, lockedLive(wiredRoll(&tr)))

	_, _, err := runIn(t, newImage(), Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	assert.NotContains(t, tr.steps, "replace:art-2")
}

// CI passes no flag: a pipeline that was already reviewed should not have to
// say so twice.
func TestRun_LockedProductionRollsInCIWithoutAsking(t *testing.T) {
	var tr track

	f := lockedLive(wiredRoll(&tr))

	install(t, f)

	result, _, err := runIn(t, newImage(), Options{
		NonInteractive: true,
		Confirm: func(string, string) (bool, error) {
			t.Fatal("nothing may prompt without a terminal")

			return false, nil
		},
	})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "lock:art-2")
	assert.True(t, result.Locked)
}

// --lock and a locked predecessor both lock, and locking twice is not a
// no-op at the platform.
func TestRun_LockFlagDoesNotLockTheVersionTwice(t *testing.T) {
	var tr track

	install(t, lockedLive(wiredRoll(&tr)))

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(strings.Join(tr.steps, " "), "lock:art-2"))
	assert.True(t, result.Locked)
}

// A failed rollout leaves the old version serving, so reporting the workload
// as healthy afterwards would be true and entirely misleading.
func TestRun_FailedRolloutSaysTheOldVersionIsStillServing(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.waitReplace = func(_ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		started.Status = workload.ReplacementStatusFailed

		return started, errors.New("replacement rep-1 ended with status failed")
	}
	f.wait = func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		t.Fatal("a failed rollout has nothing to settle")

		return nil, nil
	}

	install(t, f)

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still running the version it was")
	assert.Contains(t, err.Error(), "dr workload logs")
	assert.Equal(t, "68a0000000000000000000a1", result.ArtifactID,
		"the envelope names what is serving, which after a failed swap is the old version")
}

func TestRun_DetachedRollDoesNotWait(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	result, stderr, err := runIn(t, newImage(), Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)

	assert.NotContains(t, tr.steps, "await-rollout")
	assert.NotContains(t, tr.steps, "settle")
	assert.Equal(t, ActionRolled, result.Action)
	assert.Contains(t, stderr, "not waiting")
}

// Rolling onto something that is not running is a different operation, and
// guessing which half the user meant is worse than asking.
func TestRun_StoppedWorkloadWithANewVersionSaysStartItFirst(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.workloadD = func(string) (workload.Document, error) {
		d := docOf(liveImageWorkloadJSON)
		d["status"] = workload.WorkloadStatusStopped

		return d, nil
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr workload start")
	assert.Empty(t, tr.steps)
}

// A new version of a built project is born with no code and no image, so it
// needs the sync and the build before anything is promoted onto it.
func TestRun_RollingABuiltProjectIsNotWiredYet(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.workloadD = func(string) (workload.Document, error) { return docOf(liveWorkloadJSON), nil }
	f.artifactD = func(string) (workload.Document, error) { return docOf(liveArtifactJSON), nil }

	install(t, f)

	drifted := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest, "port: 8000", "port: 9000", 1)

	_, stderr, err := runIn(t, drifted, Options{NonInteractive: true})
	require.ErrorIs(t, err, ErrNotWired)
	assert.Contains(t, err.Error(), "dockerfile")
	assert.Empty(t, tr.steps, "a refusal must not have created a version first")
	assert.Contains(t, stderr, "+ artifact", "the plan is still printed")
}
