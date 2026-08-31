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
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
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

	// The same artifact with the credential reference already on it. A file
	// that names the same reference therefore describes no artifact change, so
	// a run that moves only the sizing really is a retune. Splicing the
	// reference into the file alone would make it drift, and the run under test
	// would quietly become a roll.
	liveCredentialArtifactJSON = `{
  "id": "68a0000000000000000000a1",
  "name": "my-app-artifact",
  "status": "draft",
  "spec": {
    "type": "service",
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {
            "name": "primary", "primary": true, "port": 8080, "imageUri": "registry/team/app:v1",
            "environmentVars": [
              {"source": "dr-credential", "name": "OPENAI_API_KEY",
               "drCredentialId": "68b0cccc0000000000000003", "key": "apiToken"}
            ]
          }
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
		replace: func(workloadID, artifactID string, runtime json.RawMessage) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "replace:"+artifactID)
			tr.rolledRuntime = runtime

			return &workload.Replacement{ID: "rep-1", WorkloadID: workloadID, ArtifactID: artifactID}, nil
		},
		waitReplace: func(_ string, started *workload.Replacement, _, _ time.Duration,
			_ func(*workload.Replacement),
		) (*workload.Replacement, error) {
			tr.steps = append(tr.steps, "await-rollout")
			started.Status = workload.ReplacementStatusCompleted

			return started, nil
		},
		// The artifact this fake is asked to wait for is recorded rather than
		// ignored: returning "art-2" unprompted is how a wait that settled on
		// the old version read as a passing test.
		wait: func(id string, want workload.Serving, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			tr.steps = append(tr.steps, servingLabel(want))

			return &workload.Workload{
				ID: id, Name: "my-app", Status: workload.WorkloadStatusRunning,
				ArtifactID: want.ArtifactID, Endpoint: "https://app.datarobot.com/workloads/68b0/",
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

	assert.Equal(t, []string{"guard", "create-artifact", "guard", "replace:art-2", "await-rollout", "settle:art-2+drain"}, tr.steps)
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

	assert.Equal(t, []string{"guard", "guard", "replace:68b0bbbb0000000000000002", "await-rollout", "settle:68b0bbbb0000000000000002+drain"}, tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
}

// The up-front guard exists so a doomed run fails before it creates a version
// nobody will ever roll.
func TestRun_ReplacementInFlightStopsBeforeAnythingIsMade(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.guard = func(workloadID string) error {
		return fmt.Errorf("workload %s: %w (status switching)", workloadID, workload.ErrReplacementInFlight)
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a replacement is already in progress")
	assert.NotContains(t, err.Error(), "cannot tell whether",
		"a refusal travels in its own words; only a failure to find out gets the operation named for it")
	assert.Empty(t, tr.steps, "nothing may be created while a rollout is running")
}

// The other half of the roll guard's contract: a route that could not answer
// says nothing about the deploy it stopped, so the operation is named for it.
func TestRun_RollNamesWhatItStoppedWhenTheGuardCannotAnswer(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.guard = func(string) error { return errors.New("HTTP error: 500 Internal Server Error") }

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"cannot tell whether workload 68b0c1d2e3f4a5b6c7d8e9f0 already has a rollout in progress, "+
			"so nothing was built or rolled out")
	assert.Contains(t, err.Error(), "500 Internal Server Error")
	assert.Empty(t, tr.steps, "a guard that could not answer is not permission to proceed")
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
		[]string{"guard", "create-artifact", "guard", "lock:art-2", "replace:art-2", "await-rollout", "settle:art-2+drain"},
		tr.steps, "the successor is locked after the last guard and before the swap")
	assert.Contains(t, asked, "is on a locked version")
	assert.NotContains(t, asked, "production", "why a version was locked is not something the CLI knows")
	assert.Equal(t, "my-app", expect, "the name is what has to be typed")
	assert.True(t, result.Locked)
}

// A file naming an artifact by id is the one way to reach matchLock with a
// candidate this run did not create, so it is the only path that asks the
// platform whether the successor is locked already. Locking twice is not a
// no-op there, which is what the question is for.
func TestRun_LockedRollDoesNotRelockAnArtifactTheFileNamed(t *testing.T) {
	var tr track

	f := lockedLive(wiredRoll(&tr))
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("the file named the version to roll onto")

		return nil, nil
	}
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		tr.steps = append(tr.steps, "read:"+id)

		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
	}

	install(t, f)

	named := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundArtifactManifest

	result, _, err := runIn(t, named, Options{
		Confirm: func(string, string) (bool, error) { return true, nil },
	})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"guard", "guard", "read:68b0bbbb0000000000000002",
		"replace:68b0bbbb0000000000000002", "await-rollout", "settle:68b0bbbb0000000000000002+drain",
	}, tr.steps, "an artifact already locked is read, not locked again")
	assert.True(t, result.Locked)
}

// The other half: a named artifact that is still a draft has to be locked to
// match the production version it is replacing, because the platform refuses
// to put a draft where a locked one was.
func TestRun_LockedRollLocksANamedDraft(t *testing.T) {
	var tr track

	f := lockedLive(wiredRoll(&tr))
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		tr.steps = append(tr.steps, "read:"+id)

		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}

	install(t, f)

	named := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundArtifactManifest

	result, _, err := runIn(t, named, Options{
		Confirm: func(string, string) (bool, error) { return true, nil },
	})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"guard", "guard", "read:68b0bbbb0000000000000002", "lock:68b0bbbb0000000000000002",
		"replace:68b0bbbb0000000000000002", "await-rollout", "settle:68b0bbbb0000000000000002+drain",
	}, tr.steps)
	assert.True(t, result.Locked)
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

		return fmt.Errorf("workload wl-1: %w (status switching)", workload.ErrReplacementInFlight)
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{
		Confirm: func(string, string) (bool, error) { return true, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a replacement is already in progress")
	assert.NotContains(t, err.Error(), "cannot tell whether")
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
	assert.Contains(t, err.Error(), "as it was")
	assert.NotContains(t, tr.steps, "lock:art-2", "locking cannot be undone, so it must not happen on a no")
	assert.NotContains(t, tr.steps, "replace:art-2")
	assert.NotContains(t, tr.steps, "create-artifact",
		"the question is put before anything is made, so a no leaves no draft behind")
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
// say so twice. There is nobody to ask, so the command passes no Confirm.
func TestRun_LockedProductionRollsInCIWithoutAsking(t *testing.T) {
	var tr track

	f := lockedLive(wiredRoll(&tr))

	install(t, f)

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "lock:art-2")
	assert.True(t, result.Locked)
}

// --output json makes the run non-interactive so no wizard is drawn into the
// document, but it is a statement about stdout, not consent. Somebody is
// still sitting there, so production still gets to be asked about; the
// question and its answer never touch stdout.
func TestRun_JSONOutputStillAsksBeforeRollingProduction(t *testing.T) {
	var (
		tr    track
		asked string
	)

	install(t, lockedLive(wiredRoll(&tr)))

	_, _, err := runIn(t, newImage(), Options{
		NonInteractive: true,
		Confirm: func(question, _ string) (bool, error) {
			asked = question

			return false, nil
		},
	})
	require.Error(t, err, "the answer was no, so nothing may roll")

	assert.Contains(t, asked, "is on a locked version", "being unable to draw a wizard is not consent")
	assert.NotContains(t, asked, "production", "why a version was locked is not something the CLI knows")
	assert.NotContains(t, tr.steps, "lock:art-2")
	assert.NotContains(t, tr.steps, "replace:art-2")
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

// The roll label leans on result.ArtifactID still naming the outgoing version
// at this point. That is true today by settle's ordering, but it is implicit:
// a reorder that updated the envelope earlier would silently restore the
// wording this change replaced.
func TestRun_RollSaysItIsWaitingForTheNewVersion(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	_, stderr, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "Waiting for the new version to serve")
	assert.NotContains(t, stderr, "Waiting for the workload to run",
		"the workload was running before this deploy started; saying so proves nothing")
}

// Both waits a roll needs can now run for minutes, so --poll-timeout has to
// bound the deploy rather than each half of it.
func TestRun_TheTwoRollWaitsShareOnePollBudget(t *testing.T) {
	var tr track

	var settleTimeout time.Duration

	f := wiredRoll(&tr)
	f.waitReplace = func(_ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		tr.steps = append(tr.steps, "await-rollout")
		started.Status = workload.ReplacementStatusCompleted

		return started, nil
	}
	f.wait = func(id string, want workload.Serving, _, timeout time.Duration,
		_ func(*workload.Workload),
	) (*workload.Workload, error) {
		settleTimeout = timeout

		return &workload.Workload{
			ID: id, Name: "my-app", Status: workload.WorkloadStatusRunning,
			ArtifactID: want.ArtifactID,
		}, nil
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true, PollTimeout: 10 * time.Minute})
	require.NoError(t, err)

	assert.Positive(t, settleTimeout)
	assert.LessOrEqual(t, settleTimeout, 10*time.Minute,
		"the second wait may not start a fresh copy of the budget the first one was already spending")
}

// The sharpest consequence of a wait that could settle early. On a draft
// workload --lock is applied after the swap, to result.ArtifactID, which is
// whatever the wait last saw. A wait that returned on the version being rolled
// off locked the wrong artifact, and locking is one-way.
func TestRun_DraftRollLocksTheVersionItRolledOnto(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{
			"guard", "create-artifact", "guard", "replace:art-2",
			"await-rollout", "settle:art-2+drain", "lock:art-2",
		}, tr.steps,
		"the lock follows the wait, so the wait is what decides which artifact becomes permanent")
	assert.NotContains(t, tr.steps, "lock:68a0000000000000000000a1",
		"locking the version being rolled off cannot be undone")
	assert.Equal(t, "art-2", result.ArtifactID)
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
	f.wait = func(string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
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

// The production confirm is asked before anything is touched, which means it is
// now asked of a workload that is switched off. It must not open by telling the
// reader their workload is running: that is the one prompt where being wrong
// about live state costs something, and the run is about to start it.
func TestRun_LockedProductionPromptDoesNotClaimAStoppedWorkloadIsRunning(t *testing.T) {
	var (
		tr    track
		asked string
	)

	f := lockedLive(stoppedRoll(&tr))

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{
		Confirm: func(question, _ string) (bool, error) {
			asked = question

			return false, nil
		},
	})
	require.Error(t, err)

	assert.NotContains(t, asked, "is running a locked version")
	assert.Contains(t, asked, "is on a locked version")
	assert.Contains(t, asked, "this deploy starts it",
		"that the run also switches the workload on is material to the answer")
	assert.NotContains(t, asked, "production",
		"a lock makes an artifact immutable; why it was taken is not something the CLI knows")
	assert.Contains(t, asked, "will be locked too, permanently",
		"permanence is stated of the lock; a bare \"this\" would point at the deploy one line above")
	assert.Equal(t, []string{"guard"}, tr.steps,
		"a no leaves the workload off and nothing minted; only the checks ahead of it ran")
}

// The same prompt against a running workload says nothing about starting,
// because nothing is being started.
func TestRun_LockedProductionPromptSaysNothingAboutStartingWhenItIsUp(t *testing.T) {
	var (
		tr    track
		asked string
	)

	install(t, lockedLive(wiredRoll(&tr)))

	_, _, err := runIn(t, newImage(), Options{
		Confirm: func(question, _ string) (bool, error) {
			asked = question

			return false, nil
		},
	})
	require.Error(t, err)

	assert.Contains(t, asked, "is on a locked version")
	assert.NotContains(t, asked, "starts it")
}

// A stopped workload with a new version to roll is started first and then
// rolled, in one run. It used to be refused, on the grounds that coming up on
// the old version and then failing was the worst of both; the refusal was
// worse, because it left the workload switched off and asked for two more
// commands. From the start onwards this is the same deploy a running workload
// gets, which is why the step list below is a roll's with a start in front.
func TestRun_StoppedWorkloadWithANewVersionStartsThenRolls(t *testing.T) {
	var tr track

	f := stoppedRoll(&tr)

	install(t, f)

	result, stderr, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"guard", "start", "await-start", "guard", "create-artifact", "guard",
		"replace:art-2", "await-rollout", "settle:art-2+drain",
	}, tr.steps, "the guard that can refuse comes before the start that mutates")
	assert.Equal(t, ActionRolled, result.Action,
		"rolling is the more significant of the two things this run did")
	assert.Contains(t, stderr, "started, having been stopped")
	assert.Contains(t, stderr, "artifact")
}

// The sizing half of the same rule: a stopped workload whose file moved only a
// resource figure is started and then resized, rather than refused for asking
// for more than a start.
func TestRun_StoppedWorkloadWithASizingChangeStartsThenRetunes(t *testing.T) {
	var tr track

	f := stoppedRoll(&tr)
	f.settings = func(id string, runtime json.RawMessage) (*workload.Replacement, error) {
		tr.steps = append(tr.steps, "settings")
		tr.rolledRuntime = runtime

		return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
	}

	install(t, f)

	resized := strings.Replace(boundImageManifest, "replicaCount: 1", "replicaCount: 2", 1)

	result, _, err := runIn(t, resized, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"guard", "start", "await-start", "guard", "settings", "await-rollout", "settle:+drain"},
		tr.steps, "the guard that can refuse comes before the start that mutates")
	assert.Equal(t, ActionUpdated, result.Action)
}

// A resize of a running workload deliberately skips the credential check, so
// that a reference this deploy does not touch cannot refuse it. A stopped one
// is not that run: it is started too, and a start is when the container
// resolves its references from cold.
//
// The live artifact carries the reference as well as the file, which is what
// makes this a retune. An earlier version of this test spliced the reference
// into the file alone; that is artifact drift, so the run was a roll, the
// settings seam was never called, and the rule the test is named for had no
// coverage at all.
func TestRun_StoppedRetuneStillVerifiesCredentials(t *testing.T) {
	var (
		tr      track
		lookups int
	)

	f := stoppedRoll(&tr)
	f.artifactD = func(string) (workload.Document, error) { return docOf(liveCredentialArtifactJSON), nil }
	f.settings = func(id string, _ json.RawMessage) (*workload.Replacement, error) {
		tr.steps = append(tr.steps, "settings")

		return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
	}
	f.cred = func(string) (*workload.Credential, error) {
		lookups++

		return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
	}

	install(t, f)

	resized := strings.Replace(withCredential(boundImageManifest), "replicaCount: 1", "replicaCount: 2", 1)

	_, _, err := runIn(t, resized, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Equal(t, 1, lookups)
	assert.Empty(t, tr.steps, "a bad reference stops the run before it starts anything")
}

// The pairing that gives the test above its meaning: the same manifest against
// a running workload is the one case that may skip the check, because a resize
// sends no environment and the artifact keeps every variable it has.
func TestRun_RunningRetuneWithACredentialSkipsTheCheck(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.artifactD = func(string) (workload.Document, error) { return docOf(liveCredentialArtifactJSON), nil }
	f.settings = func(id string, _ json.RawMessage) (*workload.Replacement, error) {
		tr.steps = append(tr.steps, "settings")

		return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
	}
	f.cred = func(string) (*workload.Credential, error) {
		t.Fatal("a settings update sends no environment, so a credential must not be able to refuse it")

		return nil, nil
	}

	install(t, f)

	resized := strings.Replace(withCredential(boundImageManifest), "replicaCount: 1", "replicaCount: 2", 1)

	_, _, err := runIn(t, resized, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Contains(t, tr.steps, "settings")
}

// The lock is about the artifact left serving, so a run that starts a workload
// only to roll it off that version must not lock what it is about to replace.
// Locking cannot be undone, so this is not a tidiness point.
func TestRun_StoppedRollLocksOnlyTheVersionItLeavesServing(t *testing.T) {
	var tr track

	install(t, stoppedRoll(&tr))

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"lock:art-2"}, locks(tr.steps),
		"one lock, on the version the run left running")
	assert.True(t, result.Locked)
}

// --detach says not to wait for the deploy. The start is not the deploy: the
// rollout cannot be requested against a workload that is not up yet, so the
// wait happens and the run says why rather than blocking in silence.
func TestRun_DetachedStoppedRollWaitsForTheStartOnly(t *testing.T) {
	var tr track

	install(t, stoppedRoll(&tr))

	result, stderr, err := runIn(t, newImage(), Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"guard", "start", "await-start", "guard", "create-artifact", "guard", "replace:art-2"},
		tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
	assert.Contains(t, stderr, "--detach applies to the deploy")
	assert.Contains(t, stderr, "not waiting")
}

// A credential id is a round trip each, and a stopped workload with drift
// passes through both the branch that starts it and the one that rolls it.
func TestRun_StoppedRollVerifiesCredentialsOnce(t *testing.T) {
	var (
		tr      track
		lookups int
	)

	f := stoppedRoll(&tr)
	f.cred = func(id string) (*workload.Credential, error) {
		lookups++

		return &workload.Credential{CredentialID: id}, nil
	}

	install(t, f)

	_, _, err := runIn(t, withCredential(newImage()), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, 1, lookups, "the same credential must not be looked up once per branch")
}

// stoppedRoll is wiredRoll against a workload that is switched off, with the
// start wired so the order of the two halves is assertable.
func stoppedRoll(tr *track) fakes {
	f := wiredRoll(tr)

	f.workloadD = func(string) (workload.Document, error) {
		d := docOf(liveImageWorkloadJSON)
		d["status"] = workload.WorkloadStatusStopped

		return d, nil
	}

	f.start = func(id string) (*workload.WorkloadOperationResponse, error) {
		tr.steps = append(tr.steps, "start")

		return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
	}

	// The two waits are the same seam, so the label says which one this is:
	// the first is the prerequisite start, the rest are the deploy settling.
	//
	// The artifact each is told to expect rides along in the label: a start
	// carries none, only the roll that follows names one.
	settled := false
	f.wait = func(id string, want workload.Serving, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
		step := "await-start"
		artifact := "68a0000000000000000000a1"

		if settled {
			step = servingLabel(want)

			if want.ArtifactID != "" {
				artifact = want.ArtifactID
			}
		}

		settled = true

		tr.steps = append(tr.steps, step)

		return &workload.Workload{
			ID: id, Name: "my-app", Status: workload.WorkloadStatusRunning,
			ArtifactID: artifact, Endpoint: "https://app.datarobot.com/workloads/68b0/",
		}, nil
	}

	return f
}

// withCredential hangs a stored-credential reference off the primary
// container, which is the one thing in a manifest no local check can judge.
func withCredential(file string) string {
	return strings.Replace(file, "imageUri: registry/team/app:",
		"environmentVars:\n"+
			"              - name: OPENAI_API_KEY\n"+
			"                value: dr-credential:68b0cccc0000000000000003/apiToken\n"+
			"            imageUri: registry/team/app:", 1)
}

// locks picks the lock steps out of a track, so a test about how many locks a
// run took does not have to restate every other step to say so.
func locks(steps []string) []string {
	var out []string

	for _, step := range steps {
		if strings.HasPrefix(step, "lock:") {
			out = append(out, step)
		}
	}

	return out
}

// builtRoll is a roll of a project whose image the platform builds and whose
// code has changed, every step wired to succeed. The live artifact is a draft
// so no lock confirmation lands among the build assertions.
func builtRoll(tr *track) fakes {
	f := wiredRoll(tr)

	f.code = func(Loaded, Live) (CodeChange, error) {
		return CodeChange{Applies: true, Files: 2}, nil
	}

	f.workloadD = func(string) (workload.Document, error) { return docOf(liveWorkloadJSON), nil }
	f.artifactD = func(string) (workload.Document, error) {
		d := docOf(liveArtifactJSON)
		d["status"] = workload.ArtifactStatusDraft

		return d, nil
	}

	f.link = func(_ string, opts wapi.InitOptions) error {
		tr.steps = append(tr.steps, "link")
		tr.linkedTo = opts

		return nil
	}
	f.save = relinkStep(tr)
	f.codeRef = carryCodeStep(tr)
	f.sync = func(string) (*sync.Result, error) {
		tr.steps = append(tr.steps, "sync")

		return &sync.Result{UploadedCount: 2, NewVersion: "ver-2"}, nil
	}
	f.build = func(string) (*workload.BuildTriggerResponse, error) {
		tr.steps = append(tr.steps, "build")

		return &workload.BuildTriggerResponse{BuildIDs: []string{"bld-2"}}, nil
	}
	f.waitBuild = func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: id, Status: workload.BuildStatusCompleted}, nil
	}

	return f
}

// builtDrift is the built project's file asking for one thing the live spec
// does not say, which is the smallest change that mints a version.
func builtDrift() string {
	return "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest, "port: 8000", "port: 9000", 1)
}

// emptySync is a sync that found nothing to do, which is the precondition of
// every test about carrying code onto a version that has none of its own.
func emptySync(tr *track) func(string) (*sync.Result, error) {
	return func(string) (*sync.Result, error) {
		tr.steps = append(tr.steps, "sync")

		return &sync.Result{}, nil
	}
}

// relinkStep records a project re-pointed at a new version, and carryCodeStep
// the patch that gives that version the code the old one was running. Both
// fixtures are shared with the create path's tests, so the step names the
// suite pins stay in one place.
func relinkStep(tr *track) func(string, wapi.Config) error {
	return func(_ string, cfg wapi.Config) error {
		tr.steps = append(tr.steps, "relink")
		tr.savedCfg = cfg

		return nil
	}
}

func carryCodeStep(tr *track) func(string, string, string) error {
	return func(artifactID, catalogID, versionID string) error {
		tr.steps = append(tr.steps, "carry-code")
		tr.carried = []string{artifactID, catalogID, versionID}

		return nil
	}
}

// syncedProject is a project that has pushed code before, linked to the
// artifact named. It is what every roll of a built project starts from,
// because the version now serving got its code the same way.
func syncedProject(artifactID string) func(string) (wapi.Config, error) {
	catalog, version := "cat1", "ver1"

	return func(string) (wapi.Config, error) {
		return wapi.Config{
			ArtifactID:          artifactID,
			CatalogID:           &catalog,
			LastSyncedVersionID: &version,
		}, nil
	}
}

// A new version of a built project is born with neither code nor an image, so
// the roll has to fill it before anything is promoted onto it. The order is
// the whole point: the project is pointed at the new version before the sync,
// or the upload would rewrite the code of the version currently serving.
func TestRun_RollsABuiltProjectOntoAFreshlyBuiltVersion(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")

	install(t, f)

	result, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"guard", "create-artifact", "relink", "sync", "build", "guard", "replace:art-2", "await-rollout", "settle:art-2+drain"},
		tr.steps)
	assert.Equal(t, "art-2", tr.savedCfg.ArtifactID, "the sync has to land in the new version, not the live one")
	assert.Equal(t, ActionRolled, result.Action)
	assert.Equal(t, "bld-2", result.BuildID, "the envelope has to be able to name the build")
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", result.WorkloadID)
}

// A built project whose live version is locked, and whose link therefore
// points at something immutable. Nothing here is new machinery. The roll
// already mints a version, moves the link and locks to match, so what is being
// asserted is that the run reaches any of it, which a preflight refusal
// computing the file count used to prevent.
func TestRun_RollsABuiltProjectOffALockedVersion(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.artifactD = func(string) (workload.Document, error) { return docOf(liveArtifactJSON), nil }
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")

	install(t, f)

	result, stderr, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"guard", "create-artifact", "relink", "sync", "build",
		"guard", "lock:art-2", "replace:art-2", "await-rollout", "settle:art-2+drain",
	}, tr.steps)
	assert.Equal(t, "art-2", tr.savedCfg.ArtifactID, "the link has to leave the locked version behind")
	assert.True(t, result.Locked, "a locked version is replaced by a locked one")
	assert.Contains(t, stderr, "the running version is locked, so a new one is created and permanently locked",
		"a --yes run locks something without being asked to, and the plan has to say so")
}

// The project is linked to the version that is serving, which is the state a
// finished roll leaves behind. Pushing into it would rewrite production's
// code in place.
func TestRun_RollNeverPushesIntoTheVersionThatIsServing(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact", "the live version is not a candidate to push into")
	assert.NotEqual(t, "68a0000000000000000000a1", tr.savedCfg.ArtifactID)
}

// A run that died between creating a version and promoting it left a draft
// with the code already in it. Making another would pile up an artifact per
// attempt, and the one left behind is exactly what this run wants.
func TestRun_RollReusesTheVersionAnEarlierAttemptLeft(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	// The leftover was made from the file this run is reading, so it says
	// what the file says. That is what makes it the version to roll onto.
	f.artifactD = artifactDocs("art-abandoned", 9000)
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("an unpromoted draft is already the version to roll onto")

		return nil, nil
	}

	install(t, f)

	result, stderr, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"guard", "sync", "build", "guard", "replace:art-abandoned", "await-rollout", "settle:art-abandoned+drain"},
		tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
	assert.Contains(t, stderr, "earlier attempt")
}

// The same reuse against the shape the platform actually answers with. The
// file states the artifact type inside the spec, which the platform accepts;
// it hoists the discriminator and returns it beside the spec, so the two
// blocks disagree about where the type lives. Unreconciled, that read as a
// field the draft was missing, no leftover ever described the file, and every
// attempt minted another one: the same drift the plan had, on the path whose
// whole job is to stop the pile.
//
// The package's other fixtures answer with the type inside the spec, which is
// not what staging returns, which is why nothing else here catches it.
func TestRun_LeftoverDraftIsReusedWhenThePlatformHoistsTheType(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("the leftover says what the file says, wherever each of them keeps the type")

		return nil, nil
	}

	// The platform's own shape: type beside the spec, never inside it.
	docs := artifactDocs("art-abandoned", 9000)
	f.artifactD = func(id string) (workload.Document, error) {
		d, err := docs(id)
		if err != nil {
			return nil, err
		}

		spec, _ := d["spec"].(map[string]any)
		delete(spec, "type")

		d["type"] = "service"

		return d, nil
	}

	install(t, f)

	result, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"guard", "sync", "build", "guard", "replace:art-abandoned", "await-rollout", "settle:art-abandoned+drain"},
		tr.steps)
	assert.Equal(t, ActionRolled, result.Action)
}

// Normalising where the type lives is not enough on its own: the walk that
// follows compares by exact equality, so a file spelling it "Service" against
// a live "service", or a document carrying no type at all, each read as a
// difference no deploy can settle. The leftover is refused, another draft is
// minted, and the next attempt refuses that one too.
func TestRun_LeftoverDraftIsReusedAcrossTypeSpellingAndSilence(t *testing.T) {
	for name, live := range map[string]func(workload.Document){
		"different spelling": func(d workload.Document) { d["type"] = "Service" },
		"no type at all":     func(d workload.Document) { delete(d, "type") },
	} {
		t.Run(name, func(t *testing.T) {
			var tr track

			f := builtRoll(&tr)
			f.linked = func(string) bool { return true }
			f.project = syncedProject("art-abandoned")
			f.getArtifact = func(id string) (*workload.Artifact, error) {
				return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
			}
			f.newArtifact = func(any) (*workload.Artifact, error) {
				t.Fatal("the leftover says what the file says; a spelling is not a different kind")

				return nil, nil
			}

			docs := artifactDocs("art-abandoned", 9000)
			f.artifactD = func(id string) (workload.Document, error) {
				d, err := docs(id)
				if err != nil {
					return nil, err
				}

				spec, _ := d["spec"].(map[string]any)
				delete(spec, "type")
				live(d)

				return d, nil
			}

			install(t, f)

			result, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
			require.NoError(t, err)
			assert.Equal(t, ActionRolled, result.Action)
		})
	}
}

// A rollout that starts and then ends failed never promotes, so the previous
// version keeps serving. Reporting "rolled" would name an outcome the workload
// did not reach, on the one field a caller reads to find out what happened.
func TestRun_FailedRolloutDoesNotReportThatItRolled(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)
	f.waitReplace = func(_ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		started.Status = workload.ReplacementStatusFailed

		return started, errors.New("rollout failed")
	}

	install(t, f)

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still running the version it was")
	assert.Equal(t, ActionUnchanged, result.Action,
		"the version serving did not change, so the run did not roll")
}

// An artifact's spec is fixed when it is created, so a draft left by an
// attempt that read an older file describes that older file. Promoting it
// would roll out a version the yaml no longer asks for and report the deploy
// as done, leaving the same drift to be found again next run.
func TestRun_RollDoesNotReuseADraftTheFileHasMovedPast(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	// The leftover still carries the port the file had when it was made; the
	// file has since asked for another.
	f.artifactD = artifactDocs("art-abandoned", 8500)

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact",
		"a draft describing an older file is not the version to roll onto")
	assert.NotContains(t, tr.steps, "replace:art-abandoned")
}

// Deleting a line is a change like any other, and the leftover was made
// before it. Reuse driven only by what the file still mentions would find
// everything matching, promote the draft, and leave the deleted variable
// running; minting a fresh version drops it. The same file has to produce the
// same running spec whether or not an earlier attempt left something behind.
func TestRun_RollDoesNotReuseADraftCarryingWhatTheFileDeleted(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}

	// The leftover agrees with the file about everything the file still says,
	// and carries one variable the file no longer does.
	f.artifactD = func(id string) (workload.Document, error) {
		d := docOf(liveArtifactJSON)
		d["status"] = workload.ArtifactStatusDraft

		if id != "art-abandoned" {
			return d, nil
		}

		container := primaryOf(d)
		container["port"] = float64(9000)
		container["environmentVars"] = []any{
			map[string]any{"name": "FOO", "value": "1"},
		}

		group, _ := d["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)
		group["containers"] = []any{container}

		return d, nil
	}

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact",
		"a draft carrying a variable the file deleted is not the version to roll onto")
	assert.NotContains(t, tr.steps, "replace:art-abandoned")
}

// artifactDocs answers the document read: the leftover draft at the port it
// was created with, and the live artifact for everything else.
//
// The leftover is built to look like what createVersion makes, which is the
// file's artifact block and nothing else. The live artifact carries a sidecar
// the file has never heard of, and that is right for the live one: an
// unmentioned sidecar is not drift. A leftover cannot have one, because the
// only thing that could have made it is this file.
func artifactDocs(abandonedID string, port int) func(string) (workload.Document, error) {
	return func(id string) (workload.Document, error) {
		d := docOf(liveArtifactJSON)
		d["status"] = workload.ArtifactStatusDraft

		if id != abandonedID {
			return d, nil
		}

		container := primaryOf(d)
		container["port"] = float64(port)

		group, _ := d["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)
		group["containers"] = []any{container}

		return d, nil
	}
}

// primaryOf reaches the first container of the first group.
func primaryOf(d workload.Document) map[string]any {
	groups, _ := d["spec"].(map[string]any)["containerGroups"].([]any)
	container, _ := groups[0].(map[string]any)["containers"].([]any)[0].(map[string]any)

	return container
}

// A locked draft can take neither code nor an image, which is what a roll
// onto production leaves behind when it dies between the lock and the swap.
// Reusing it would fail at the push with the platform's own 403.
func TestRun_RollDoesNotReuseALockedLeftover(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
	}

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact", "nothing more can be pushed into a locked draft")
	assert.Contains(t, tr.steps, "replace:art-2")
}

// A roll whose sync uploads nothing leaves the new version with no code of its
// own, and the plan's file count is only a prediction.
func TestRun_RollWithNothingToUploadCarriesTheCodeOver(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")
	f.sync = emptySync(&tr)

	install(t, f)

	result, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"art-2", "cat1", "ver1"}, tr.carried,
		"a new version of the same code has to point at that code")
	assert.Equal(t, []string{
		"guard", "create-artifact", "relink", "sync", "carry-code", "build",
		"guard", "replace:art-2", "await-rollout", "settle:art-2+drain",
	}, tr.steps)
	assert.Equal(t, "bld-2", result.BuildID, "a version born without an image still needs one")
}

// A version with no code must not reach the builder, nor be promoted.
func TestRun_RollWithNoCodeToCarryStopsBeforeTheBuild(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) {
		return wapi.Config{ArtifactID: "68a0000000000000000000a1"}, nil
	}
	f.sync = emptySync(&tr)

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-2")
	assert.Contains(t, err.Error(), "nothing to build")
	assert.NotContains(t, tr.steps, "build")
	assert.NotContains(t, tr.steps, "replace:art-2")
}

// The version exists and only the local record of it failed to write. The
// next run would create another and this one would be unreachable, so the
// error has to carry the id.
func TestRun_RollLinkFailureNamesTheStrandedVersion(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")
	f.save = func(string, wapi.Config) error { return errors.New("permission denied") }

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-2")
	assert.Contains(t, err.Error(), "artifactId",
		"the directory is already linked, so 'dr artifact code init' would refuse: name the file to edit")
	assert.Contains(t, err.Error(), "config.json")
	assert.NotContains(t, tr.steps, "sync", "nothing may be pushed at a version nothing points to")
}

// A build that fails ends the roll with the old version still serving, and
// the id of the build is the only way to the logs that say why.
func TestRun_FailedBuildOnARollLeavesTheOldVersionServing(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")
	// WaitForBuild's own contract: a terminal failure comes back as the build
	// and an error together.
	f.waitBuild = func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: id, Status: workload.BuildStatusFailed},
			fmt.Errorf("build %s ended with status %s", id, workload.BuildStatusFailed)
	}

	install(t, f)

	result, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr artifact build logs")
	assert.Equal(t, "bld-2", result.BuildID, "a failed build is still the build to go and read")
	assert.NotContains(t, tr.steps, "replace:art-2")
	assert.Equal(t, "68a0000000000000000000a1", result.ArtifactID,
		"the envelope names what is serving, which is still the old version")
}

// copiedArtifact is what a clone comes back as: its source's image and code.
// The build rows stayed on the source.
func copiedArtifact() *workload.Artifact {
	primary := true

	return &workload.Artifact{
		ID:     "art-2",
		Status: workload.ArtifactStatusDraft,
		Spec: workload.Spec{
			ContainerGroups: []workload.ContainerGroup{{
				Containers: []workload.Container{{
					Name:     "vllm-server",
					Primary:  &primary,
					ImageURI: "registry.internal/built-by-the-server:sha-abc123",
					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{
							Datarobot: &workload.DatarobotCodeRef{CatalogID: "cat1", CatalogVersionID: "ver1"},
						},
					},
				}},
			}},
		},
	}
}

func copyStep(tr *track, copied *workload.Artifact) func(string, string) (*workload.Artifact, error) {
	return func(sourceID, name string) (*workload.Artifact, error) {
		tr.steps = append(tr.steps, "copy:"+sourceID)
		tr.copiedAs = name
		copied.Name = name

		return copied, nil
	}
}

func updateSpecStep(tr *track) func(string, json.RawMessage) error {
	return func(artifactID string, spec json.RawMessage) error {
		tr.steps = append(tr.steps, "update-spec:"+artifactID)
		tr.updatedSpec = spec

		return nil
	}
}

// readBackStep answers the read after the write the way the platform does: the
// written spec with the server-owned image put back. Anything that is not the
// copy is the version now serving.
func readBackStep(tr *track, copied *workload.Artifact, live func() workload.Document) func(string) (workload.Document, error) {
	return func(id string) (workload.Document, error) {
		if id != copied.ID || tr.updatedSpec == nil {
			return live(), nil
		}

		var spec map[string]any

		if err := json.Unmarshal(tr.updatedSpec, &spec); err != nil {
			return nil, err
		}

		doc := workload.Document{"id": copied.ID, "status": copied.Status, "spec": spec}

		if uri := workload.GetPrimaryContainerImageURI(*copied); uri != "" {
			workload.PrimaryContainerInDocument(doc)["imageUri"] = uri
		}

		return doc, nil
	}
}

func draftLiveArtifact() workload.Document {
	d := docOf(liveArtifactJSON)
	d["status"] = workload.ArtifactStatusDraft

	return d
}

func runtimeOnlyRoll(tr *track) fakes {
	return runtimeOnlyRollOf(tr, copiedArtifact())
}

func runtimeOnlyRollOf(tr *track, copied *workload.Artifact) fakes {
	f := builtRoll(tr)

	f.code = func(Loaded, Live) (CodeChange, error) { return CodeChange{Applies: true}, nil }
	f.sync = emptySync(tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("68a0000000000000000000a1")
	f.copyArtifact = copyStep(tr, copied)
	f.artifactD = readBackStep(tr, copied, draftLiveArtifact)
	f.updateSpec = updateSpecStep(tr)
	f.deleteArtifact = func(id string) error {
		tr.steps = append(tr.steps, "delete:"+id)

		return nil
	}

	return f
}

func envDrift() string {
	return "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest,
			"            port: 8000\n",
			"            port: 8000\n"+
				"            environmentVars:\n"+
				"              - name: LOG_LEVEL\n"+
				"                value: debug\n", 1)
}

func buildDrift() string {
	return "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest,
			"                source: provided\n",
			"                source: generated\n"+
				"                executionEnvironmentId: 6890000000000000000000e1\n"+
				"                executionEnvironmentVersionId: 6890000000000000000000e2\n"+
				"                entrypoint: [\"python\", \"app.py\"]\n", 1)
}

// The deploy this whole path exists for. The copy's code reference has to
// survive the write, since the manifest states none.
// Production is the same deploy plus the lock, and is where the saving is worth
// most: the platform checks image provenance across the tenant, precisely so a
// copy can still be locked.
func TestRun_RuntimeOnlyRollCopiesTheRunningVersion(t *testing.T) {
	for _, c := range []struct {
		name  string
		live  func() workload.Document
		steps []string
	}{
		{
			name:  "a draft is replaced by a draft",
			live:  draftLiveArtifact,
			steps: []string{"guard", "replace:art-2", "await-rollout", "settle:art-2+drain"},
		},
		{
			name:  "a locked one is replaced by a locked one",
			live:  func() workload.Document { return docOf(liveArtifactJSON) },
			steps: []string{"guard", "lock:art-2", "replace:art-2", "await-rollout", "settle:art-2+drain"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var tr track

			f := runtimeOnlyRoll(&tr)
			f.artifactD = readBackStep(&tr, copiedArtifact(), c.live)

			install(t, f)

			result, _, err := runIn(t, envDrift(), Options{NonInteractive: true})
			require.NoError(t, err)

			assert.Equal(t, append([]string{
				"guard", "copy:68a0000000000000000000a1", "update-spec:art-2", "relink", "sync",
			}, c.steps...), tr.steps)
			assert.Empty(t, result.BuildID, "a version that inherits an image has none to build")
			assert.Equal(t, ActionRolled, result.Action)
			assert.Equal(t, "art-2", tr.savedCfg.ArtifactID, "the project pushes to the version it made")
			assert.Equal(t, "gpt-oss-20b-vllm-artifact", tr.copiedAs)

			spec := string(tr.updatedSpec)
			assert.Contains(t, spec, "LOG_LEVEL", "the change that started the run has to land")
			assert.Contains(t, spec, `"catalogId":"cat1"`, "and must not cost the copy its code reference")
			assert.Contains(t, spec, `"catalogVersionId":"ver1"`)
		})
	}
}

// The four ways a roll still pays for a build.
func TestRun_RollsThatStillBuild(t *testing.T) {
	noImage := func(f *fakes) {
		f.artifactD = func(string) (workload.Document, error) {
			d := docOf(liveArtifactJSON)
			d["status"] = workload.ArtifactStatusDraft

			delete(primaryOf(d), "imageUri")

			return d, nil
		}
	}

	stale := func(f *fakes) {
		f.code = func(Loaded, Live) (CodeChange, error) {
			return CodeChange{Applies: true, ImageStale: true}, nil
		}
	}

	cases := []struct {
		name     string
		manifest string
		force    bool
		tweak    func(*fakes)
	}{
		{name: "the dockerfile changed", manifest: buildDrift()},
		{name: "--force-build asked for one", manifest: envDrift(), force: true},
		{name: "the running version has no image", manifest: envDrift(), tweak: noImage},
		{name: "the code has moved past the running image", manifest: envDrift(), tweak: stale},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var tr track

			f := runtimeOnlyRoll(&tr)
			if c.tweak != nil {
				c.tweak(&f)
			}

			install(t, f)

			result, _, err := runIn(t, c.manifest, Options{NonInteractive: true, ForceBuild: c.force})
			require.NoError(t, err)

			assert.NotContains(t, tr.steps, "copy:68a0000000000000000000a1")
			assert.Contains(t, tr.steps, "create-artifact")
			assert.Equal(t, "bld-2", result.BuildID)
		})
	}
}

// The ways a copy goes wrong. Every one ends in the deploy that creates and
// builds, so none fails a deploy that used to work. A copy that exists is
// taken away first, unless it cannot be, where the run stops and names it.
func TestRun_CopiesThatGoWrong(t *testing.T) {
	refuse := func(status int) func(*fakes, *track, *workload.Artifact) {
		return func(f *fakes, tr *track, _ *workload.Artifact) {
			f.copyArtifact = func(id, _ string) (*workload.Artifact, error) {
				tr.steps = append(tr.steps, "copy:"+id)

				return nil, &drapi.HTTPError{StatusCode: status}
			}
		}
	}

	writeAnswers := func(err error) func(*fakes, *track, *workload.Artifact) {
		return func(f *fakes, _ *track, _ *workload.Artifact) {
			f.updateSpec = func(string, json.RawMessage) error { return err }
		}
	}

	fellBack := []string{"create-artifact", "build"}
	tookBack := []string{"delete:art-2", "create-artifact", "build"}

	cases := []struct {
		name       string
		breaks     func(*fakes, *track, *workload.Artifact)
		wantErr    string
		wantSteps  []string
		wantAbsent []string
		wantBuild  string
	}{
		// 404 is a platform with no copy endpoint and 500 is one that broke.
		// Neither left anything behind, so both take the fallback.
		{
			name: "the copy never happens", breaks: refuse(http.StatusNotFound),
			wantSteps: fellBack, wantBuild: "bld-2",
		},
		{
			name: "the write is refused", breaks: writeAnswers(&drapi.HTTPError{StatusCode: http.StatusNotFound}),
			wantSteps: tookBack, wantBuild: "bld-2",
		},
		// Accepted, and the copy still does not say what the file says.
		{
			name: "the write lands nowhere", breaks: writeAnswers(nil),
			wantSteps: tookBack, wantBuild: "bld-2",
		},
		{
			// An artifact cannot be moved between repositories, so a copy that
			// landed in one of its own would fork the version history for good.
			name: "the copy lands in another repository",
			breaks: func(f *fakes, _ *track, copied *workload.Artifact) {
				copied.ArtifactRepositoryID = "repo-other"
				f.artifactD = func(string) (workload.Document, error) {
					d := draftLiveArtifact()
					d["artifactRepositoryId"] = "repo-1"

					return d, nil
				}
			},
			wantSteps: tookBack, wantAbsent: []string{"update-spec:art-2"}, wantBuild: "bld-2",
		},
		{
			name: "the write costs the copy its image",
			breaks: func(f *fakes, tr *track, copied *workload.Artifact) {
				f.updateSpec = func(id string, spec json.RawMessage) error {
					tr.steps = append(tr.steps, "update-spec:"+id)
					tr.updatedSpec = spec
					copied.Spec.ContainerGroups[0].Containers[0].ImageURI = ""

					return nil
				}
			},
			wantSteps: []string{"build", "replace:art-2"}, wantBuild: "bld-2",
		},
		{
			name: "the copy cannot be taken away either: its id is the only record",
			breaks: func(f *fakes, _ *track, _ *workload.Artifact) {
				f.updateSpec = func(string, json.RawMessage) error { return errors.New("spec rejected") }
				f.deleteArtifact = func(string) error { return errors.New("still referenced") }
			},
			wantErr:    "left behind",
			wantAbsent: []string{"create-artifact", "replace:art-2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var tr track

			copied := copiedArtifact()
			f := runtimeOnlyRollOf(&tr, copied)
			c.breaks(&f, &tr, copied)

			install(t, f)

			result, _, err := runIn(t, envDrift(), Options{NonInteractive: true})

			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				assert.Contains(t, err.Error(), manifest.FileName,
					"a copy that cannot be made to match names the file it was measured against")
			} else {
				require.NoError(t, err)
			}

			for _, step := range c.wantSteps {
				assert.Contains(t, tr.steps, step)
			}

			for _, step := range c.wantAbsent {
				assert.NotContains(t, tr.steps, step)
			}

			assert.Equal(t, c.wantBuild, result.BuildID)
			assert.False(t, result.Plan.JSON().KeepsImage, "no run that built or failed kept an image")
		})
	}
}

func TestInheritsImage_ArtifactType(t *testing.T) {
	// The rest of inheritsImage's conditions are covered end to end by
	// TestRun_RollsThatStillBuild. Only the type comparison is here, because it
	// is the one the plan and the copy path came to answer differently.
	plan := Plan{
		Code: CodeChange{Applies: true},
		Artifact: []Change{{Keys: []string{
			keyContainerGroups, "default", keyContainers, "app", keyEnvironmentVars,
		}}},
	}
	live := Live{ArtifactID: "art-1", ImageURI: "img", ArtifactType: "agent"}

	for kind, want := range map[string]bool{
		"":        true, // a file stating no type has no opinion
		"agent":   true,
		"Agent":   true, // the platform disagrees with itself about casing
		"service": false,
	} {
		t.Run("file says "+kind, func(t *testing.T) {
			assert.Equal(t, want, inheritsImage(live, plan, Options{}, kind, "an-artifact"))
		})
	}
}

// The record the next deploy reads is written by the build that made it, and
// nothing else. A build that stopped recording would leave every later
// runtime-only deploy inheriting an image with nothing to measure.
func TestRun_RecordingWhatABuildWasMadeFrom(t *testing.T) {
	cases := []struct {
		name   string
		copies bool
		fails  bool
		want   int
	}{
		{name: "a build that succeeds records it", want: 1},
		{name: "a build that fails records nothing", fails: true},
		{name: "a deploy that skips the build records nothing", copies: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var (
				tr       track
				recorded []string
			)

			file, f := builtDrift(), builtRoll(&tr)
			if c.copies {
				file, f = envDrift(), runtimeOnlyRoll(&tr)
			}

			f.linked = func(string) bool { return true }
			f.project = syncedProject("68a0000000000000000000a1")
			f.recordBuilt = func(dir string) { recorded = append(recorded, dir) }

			if c.fails {
				f.waitBuild = func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
					return &workload.Build{ID: id, Status: workload.BuildStatusFailed},
						fmt.Errorf("build %s ended with status %s", id, workload.BuildStatusFailed)
				}
			}

			install(t, f)

			_, _, err := runIn(t, file, Options{NonInteractive: true})
			assert.Equal(t, c.fails, err != nil)
			assert.Equal(t, !c.copies, slices.Contains(tr.steps, "build"))
			assert.Len(t, recorded, c.want, "the record names the code an image was built from")
		})
	}
}

// A leftover carrying a code reference of its own is still re-anchored at the
// code the project last synced, since `dr artifact create` and an abandoned
// attempt both point at code older than the last sync. Only a leftover that
// kept the running image is left alone.
func TestRun_ALeftoverWithItsOwnCodeIsStillReAnchored(t *testing.T) {
	var tr track

	f := builtRoll(&tr)
	f.code = func(Loaded, Live) (CodeChange, error) { return CodeChange{Applies: true}, nil }
	f.sync = emptySync(&tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		primary := true

		return &workload.Artifact{
			ID:     id,
			Status: workload.ArtifactStatusDraft,
			Spec: workload.Spec{ContainerGroups: []workload.ContainerGroup{{
				Containers: []workload.Container{{
					Primary: &primary,
					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{
							Datarobot: &workload.DatarobotCodeRef{CatalogID: "cat1", CatalogVersionID: "ver0"},
						},
					},
				}},
			}}},
		}, nil
	}
	f.artifactD = artifactDocs("art-abandoned", 9000)

	install(t, f)

	_, _, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "carry-code",
		"a reference older than the last sync is not the code this deploy is for")
	assert.Contains(t, tr.steps, "build")
	assert.Equal(t, []string{"art-abandoned", "cat1", "ver1"}, tr.carried,
		"the version the project last synced is what it is pointed at")
}
