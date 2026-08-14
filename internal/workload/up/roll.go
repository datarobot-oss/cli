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
	if live.State == StateStopped {
		return result, fmt.Errorf(
			"workload %s is stopped and the file asks for more than starting it. "+
				"Start it with 'dr workload start %s' and deploy again, so the new version rolls out onto "+
				"something that is running",
			result.Name, live.WorkloadID)
	}

	// A new version of a built project is born with neither code nor an
	// image, so rolling onto it would promote something that cannot start.
	// Giving it both is the next change, and it is more than a longer wait:
	// the code has to reach the new version before the old one is touched.
	// The test is the file's build mode rather than whether code changed,
	// because a spec-only edit to a built project has the same problem.
	if mode := loaded.Manifest.BuildMode(); mode == manifest.BuildModeDockerfile || mode == manifest.BuildModeGenerated {
		return result, fmt.Errorf(
			"%w: rolling a workload whose image the platform builds (%s mode). The plan above is correct, "+
				"and a new version of a published image rolls today",
			ErrNotWired, mode)
	}

	if err := guardReplacementFn(live.WorkloadID); err != nil {
		return result, err
	}

	candidate, fresh, err := candidateArtifact(loaded, report)
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

	return replace(live.WorkloadID, candidate, lock, fresh, result, opts, report)
}

// candidateArtifact is the version to roll onto, and whether this run made
// it. A file bound to an artifact by id is asking for something that already
// exists, so there is nothing to create; minting one anyway would leave a
// stray artifact behind on every deploy.
func candidateArtifact(loaded Loaded, report *reporter) (string, bool, error) {
	if id := loaded.Compiled.ArtifactID; id != "" {
		return id, false, nil
	}

	var created *workload.Artifact

	err := report.run("Creating the new version", func() error {
		payload, payloadErr := loaded.Compiled.ArtifactPayload()
		if payloadErr != nil {
			return payloadErr
		}

		artifact, createErr := createArtifactFn(payload)
		created = artifact

		return createErr
	})
	if err != nil {
		return "", false, err
	}

	return created.ID, true, nil
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
func matchLock(candidate string, fresh bool, report *reporter) (bool, error) {
	// A candidate the file named may already be locked, and locking twice is
	// not a no-op at the platform. One created here is always a draft.
	if !fresh {
		already, lockedErr := lockedAlready(candidate)
		if lockedErr != nil {
			return false, lockedErr
		}

		if already {
			return true, nil
		}
	}

	err := report.run("Locking the new version", func() error {
		_, lockErr := lockArtifactFn(candidate)

		return lockErr
	})
	if err != nil {
		return false, fmt.Errorf(
			"artifact %s could not be locked, and a locked workload only accepts a locked version, "+
				"so nothing was rolled: %w", candidate, err)
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
	if opts.NonInteractive {
		return true, nil
	}

	if opts.Confirm == nil {
		return false, errors.New(
			"this rolls a locked, production version and there is no terminal to confirm on. " +
				"Re-run with --yes to say so explicitly")
	}

	return opts.Confirm(fmt.Sprintf(
		"Workload %s is running a locked version, which means production.\n"+
			"The new version will be locked too, and locking cannot be undone.\n"+
			"Type the workload name to roll it, anything else to stop: ", workloadName), workloadName)
}

// replace starts the rollout and follows it to the end.
func replace(
	workloadID, artifactID string,
	lock, fresh bool,
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
		locked, err := matchLock(artifactID, fresh, report)
		if err != nil {
			return result, err
		}

		result.Locked = locked
	}

	var started *workload.Replacement

	err := report.run("Rolling out the new version", func() error {
		replacement, startErr := startReplacementFn(workloadID, artifactID)
		started = replacement

		return startErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot roll workload %s onto artifact %s: %w", workloadID, artifactID, err)
	}

	result.Action = ActionRolled

	if opts.Detach {
		report.say("  Rollout of %s onto artifact %s requested; not waiting.\n", workloadID, artifactID)

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
