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

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// roll moves a live workload onto a new version of what the file describes.
// The endpoint never changes and the version already serving keeps serving
// until the new one is ready, which is what makes this safe to run against
// something in use.
//
// The order is not a preference. The guard comes first because a second
// replacement POSTed while one is in flight queues another swap rather than
// being refused, so the check that matters is the one immediately before the
// call, and an early one saves a doomed run from creating anything. The
// candidate is minted next, and the question of locking is put then, while
// the answer still costs nothing.
//
// The lock itself is last, inside replace, between the final guard and the
// POST. The platform will not replace a locked artifact with a draft, so the
// candidate has to be locked to go; but locking is one-way, and an artifact
// locked for a rollout that is then refused can be neither unlocked nor
// deleted. Taking the lock only once nothing is left that can say no is what
// keeps a lost race from leaving one behind.
func roll(loaded Loaded, live Live, plan Plan, result Result, opts Options, report *reporter) (Result, error) {
	if err := guardReplacementFn(live.WorkloadID); err != nil {
		return result, err
	}

	made, err := candidateArtifact(loaded, live, plan.Code, opts, report)

	// Recorded before the error check: a failed build is still a build, and
	// the caller's envelope should be able to name the one to go and read.
	result.BuildID = made.BuildID

	if err != nil {
		return result, err
	}

	// result.ArtifactID stays on the version that is serving, and settle
	// moves it when the rollout has actually promoted the new one. A failed
	// swap leaves the old version running, so an envelope naming the
	// candidate would send someone to read the wrong artifact.

	// Asked now, locked later. Agreeing costs nothing that cannot be undone;
	// the lock is taken inside replace, once the last guard has passed.
	lock, err := confirmLock(live, result.Name, opts)
	if err != nil {
		return result, err
	}

	// A file that moved the sizing as well as the version rides both in on the
	// same swap: the platform takes runtime alongside the artifact, so there is
	// no second call and no window where one half is live and the other is not.
	runtime, err := rollRuntime(loaded, plan)
	if err != nil {
		return result, err
	}

	return replace(live.WorkloadID, made, lock, runtime, result, opts, report)
}

// candidateArtifact is the version to roll onto.
//
// A file bound to an artifact by id is asking for one that already exists, so
// there is nothing to make; minting one anyway would leave a stray behind on
// every deploy. A file the platform builds from needs the working tree and an
// image before it is worth promoting. A published image needs neither, and is
// one call.
func candidateArtifact(
	loaded Loaded,
	live Live,
	code CodeChange,
	opts Options,
	report *reporter,
) (version, error) {
	if id := loaded.Compiled.ArtifactID; id != "" {
		return version{ID: id}, nil
	}

	if mode := loaded.Manifest.BuildMode(); mode == manifest.BuildModeDockerfile || mode == manifest.BuildModeGenerated {
		return buildVersion(loaded, live, code, opts, report)
	}

	return createVersion(loaded, report)
}

// confirmLock asks whether to roll a locked version, and reports whether the
// candidate has to be locked to match. Draft replaces draft and locked
// replaces locked; the platform rejects a mismatch.
//
// Only the question is asked here. The lock itself waits until immediately
// before the swap, because locking is one-way and an artifact locked for a
// rollout that never starts can be neither unlocked nor deleted.
func confirmLock(live Live, workloadName string, opts Options) (bool, error) {
	if !live.Locked {
		return false, nil
	}

	agreed, err := confirmRoll(workloadName, opts)
	if err != nil {
		return false, err
	}

	if !agreed {
		return false, fmt.Errorf("cancelled: workload %s is still running the version it was", workloadName)
	}

	return true, nil
}

// matchLock makes the candidate immutable, and reports whether it is.
//
// It runs between the final in-flight guard and the swap. That window is the
// whole point: the platform wants the lock in place before the replacement is
// POSTed, and by then everything that could still refuse the rollout has had
// its say, so a lock taken here is one the run is committed to using.
func matchLock(made version, report *reporter) (bool, error) {
	// A candidate the file named, or one left by an earlier attempt, may
	// already be locked, and locking twice is not a no-op at the platform.
	// One created by this run is always a draft.
	if !made.Fresh {
		already, lockedErr := lockedAlready(made.ID)
		if lockedErr != nil {
			return false, lockedErr
		}

		if already {
			return true, nil
		}
	}

	err := report.run("Locking the new version", func() error {
		_, lockErr := lockArtifactFn(made.ID)

		return lockErr
	})
	if err != nil {
		return false, fmt.Errorf(
			"artifact %s could not be locked, and a locked workload only accepts a locked version, "+
				"so nothing was rolled: %w", made.ID, err)
	}

	return true, nil
}

// lockedAlready reports whether the candidate is already immutable.
func lockedAlready(artifactID string) (bool, error) {
	artifact, err := getArtifactFn(artifactID)
	if err != nil {
		return false, fmt.Errorf("cannot read artifact %s to see whether it is already locked: %w", artifactID, err)
	}

	return artifact.IsLocked(), nil
}

// confirmRoll is decision 1 of the design, settled: `up` rolls production the
// same as any other run, and the whole ceremony is typing the name back on an
// interactive terminal. CI passes no flag, because a pipeline someone already
// reviewed should not have to say so twice.
//
// A run that cannot ask is refused rather than waved through. Being unable to
// prompt is not the same as having been told to go ahead, and the difference
// matters exactly once.
func confirmRoll(workloadName string, opts Options) (bool, error) {
	// Being able to ask settles it, ahead of any inference about whether
	// anyone is there. NonInteractive is also true for --output json, which
	// says how to format stdout rather than that production may be rolled
	// without a word, and the caller passes no Confirm when the answer is
	// already in.
	if opts.Confirm != nil {
		return opts.Confirm(fmt.Sprintf(
			"Workload %s is running a locked version, which means production.\n"+
				"The new version will be locked too, and locking cannot be undone.\n"+
				"Type the workload name to roll it, anything else to stop: ", workloadName), workloadName)
	}

	if opts.NonInteractive {
		return true, nil
	}

	return false, errors.New(
		"this rolls a locked, production version and there is no terminal to confirm on. " +
			"Re-run with --yes to say so explicitly")
}

// replace starts the rollout and follows it to the end. runtime is nil unless
// the sizing changed too, in which case it travels with the swap.
func replace(
	workloadID string,
	made version,
	lock bool,
	runtime json.RawMessage,
	result Result,
	opts Options,
	report *reporter,
) (Result, error) {
	// The guard that actually holds. The live state can have changed since
	// the one at the top, and this is the last moment before a swap that
	// cannot be taken back by refusing it.
	if err := guardReplacementFn(workloadID); err != nil {
		return result, err
	}

	if lock {
		locked, err := matchLock(made, report)
		if err != nil {
			return result, err
		}

		result.Locked = locked
	}

	var started *workload.Replacement

	err := report.run("Rolling out the new version", func() error {
		replacement, startErr := startReplacementFn(workloadID, made.ID, runtime)
		started = replacement

		return startErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot roll workload %s onto artifact %s: %w", workloadID, made.ID, err)
	}

	result.Action = ActionRolled

	if opts.Detach {
		report.say("  Rollout of %s onto artifact %s requested; not waiting.\n", workloadID, made.ID)

		return result, nil
	}

	if err := awaitRollout(workloadID, started, opts, report); err != nil {
		return result, err
	}

	return settle(workloadID, result, opts, report)
}

// awaitRollout waits for the swap itself, before the wait for the workload.
// They are two questions: the rollout says whether the new version was
// promoted, and a failed one leaves the old version serving, so reporting the
// workload as healthy afterwards would be true and completely misleading.
func awaitRollout(workloadID string, started *workload.Replacement, opts Options, report *reporter) error {
	var settled *workload.Replacement

	err := report.run("Waiting for the rollout", func() error {
		replacement, waitErr := waitReplacementFn(workloadID, started, opts.PollInterval, opts.PollTimeout, nil)
		settled = replacement

		return waitErr
	})
	if err == nil {
		return nil
	}

	if settled != nil && workload.IsFailedReplacementStatus(settled.Status) {
		return fmt.Errorf(
			"the rollout of workload %s ended as %s, so it is still running the version it was; "+
				"check 'dr workload logs %s': %w",
			workloadID, settled.Status, workloadID, err)
	}

	return err
}
