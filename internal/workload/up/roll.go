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
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// roll moves a live workload onto a new version of what the file describes.
// The endpoint never changes and the version already serving keeps serving
// until the new one is ready, which is what makes this safe to run against
// something in use.
//
// The order is not a preference. The guard comes first because a second
// replacement POSTed while one is in flight queues another swap rather than
// being refused, so the check that matters is the one immediately before the
// call, and an early one saves a doomed run from creating anything.
//
// Whether a locked version may be rolled at all is settled by the caller,
// before it. This can be reached with the workload already started, and a
// question asked after that is one whose "no" has already changed something.
//
// The lock itself is last, inside replace, between the final guard and the
// POST. The platform will not replace a locked artifact with a draft, so the
// candidate has to be locked to go; but locking is one-way, and an artifact
// locked for a rollout that is then refused can be neither unlocked nor
// deleted. Taking the lock only once nothing is left that can say no is what
// keeps a lost race from leaving one behind.
func roll(loaded Loaded, live Live, plan Plan, lock bool, result Result, opts Options, report *reporter) (Result, error) {
	if err := guardRollout(live.WorkloadID, "nothing was built or rolled out"); err != nil {
		return result, err
	}

	made, err := candidateArtifact(loaded, live, plan, opts, report)

	// Recorded before the error check: a failed build is still a build, and
	// the caller's envelope should be able to name the one to go and read.
	result.BuildID = made.BuildID

	// By here it is known rather than predicted: a leftover taken ahead of a
	// copy, a platform with no copy endpoint and a tree that moved between the
	// plan and the sync all reach this line having built anyway.
	result.Plan.InheritsImage = plan.InheritsImage && err == nil && made.BuildID == ""

	if err != nil {
		return result, err
	}

	// result.ArtifactID stays on the version that is serving, and settle moves
	// it once the rollout has promoted the new one, which it now waits to see
	// rather than assuming. A failed swap leaves the old version running, so an
	// envelope naming the candidate would point at the wrong artifact.

	// A file that moved the sizing as well as the version rides both in on the
	// same swap: the platform takes runtime alongside the artifact, so there is
	// no second call and no window where one half is live and the other is not.
	sizing, err := rollRuntime(loaded, plan)
	if err != nil {
		return result, err
	}

	return replace(live.WorkloadID, made, lock, sizing, result, opts, report)
}

// candidateArtifact is the version to roll onto.
//
// A file bound to an artifact by id is asking for one that already exists, so
// there is nothing to make; minting one anyway would leave a stray behind on
// every deploy. A file the platform builds from needs the working tree and an
// image before it is worth promoting. A published image needs neither, and is
// one call.
//
// Whatever is minted joins the repository the running version belongs to, so a
// workload deployed ten times reads as ten versions of one thing rather than
// ten artifacts that happen to share a name.
func candidateArtifact(
	loaded Loaded,
	live Live,
	plan Plan,
	opts Options,
	report *reporter,
) (version, error) {
	if id := loaded.Compiled.ArtifactID; id != "" {
		return version{ID: id}, nil
	}

	repository := sameRepository(loaded, live)

	// The same reading the plan used, rather than a second one off the parse
	// tree: two answers to "does the platform build this image" coming apart is
	// how a plan describes a deploy that does not happen.
	if plan.Code.Applies {
		return buildVersion(loaded, live, plan, repository, opts, report)
	}

	return createVersion(loaded, repository, labelNewVersion, report)
}

// sameRepository is the repository the new version joins, "" when it should
// start one of its own.
//
// The one case that has to start fresh is a file that changed the artifact's
// kind. A repository carries a type, taken from the artifact that opened it,
// and the platform refuses an artifact whose own type disagrees. A service
// that became an agent is a new lineage rather than the next version of the
// old one, so asking for the old repository would earn a 422 saying exactly
// that. Cheaper and clearer to notice it here, where both types are already in
// hand.
func sameRepository(loaded Loaded, live Live) string {
	if live.ArtifactRepositoryID == "" || !sameArtifactType(loaded, live) {
		return ""
	}

	return live.ArtifactRepositoryID
}

// sameArtifactType reports whether the file still asks for the kind of thing
// the workload is running.
//
// Both sides are defaulted, which is deliberately not what the plan does with
// the same two values. The plan treats an unstated type as the file having no
// opinion, because it never reverts what the file leaves out. The question
// here is a different one: what kind of thing is the artifact about to be
// created. The platform answers service for a block that names none, so that
// is what the repository will be compared against, whatever the file meant by
// saying nothing. The comparison itself is shared with the plan's, so the two
// cannot come to disagree about what a type change is.
//
// A file that cannot be read answers no, which starts a new repository. That
// costs a row in the Registry; joining the wrong one costs a round trip.
func sameArtifactType(loaded Loaded, live Live) bool {
	wanted, err := loaded.ArtifactType()
	if err != nil {
		return false
	}

	return manifest.SameArtifactType(
		manifest.ArtifactTypeOrDefault(wanted),
		manifest.ArtifactTypeOrDefault(live.ArtifactType),
	)
}

// confirmLock asks whether to roll a locked version, and reports whether the
// candidate has to be locked to match. Draft replaces draft and locked
// replaces locked; the platform rejects a mismatch.
//
// Only the question is asked here. The lock itself waits until immediately
// before the swap, because locking is one-way and an artifact locked for a
// rollout that never starts can be neither unlocked nor deleted.
//
// It is asked before the deploy touches anything, which is what lets the
// refusal below tell the truth: nothing has happened yet, so the workload
// really is still running what it was, and a stopped one is still stopped.
func confirmLock(live Live, workloadName string, opts Options) (bool, error) {
	if !live.Locked {
		return false, nil
	}

	agreed, err := confirmRoll(live, workloadName, opts)
	if err != nil {
		return false, err
	}

	if !agreed {
		// Not "still running the version it was": the same question is asked of
		// a stopped workload, which this run would have started, and telling
		// someone their switched-off workload is still running is worse than
		// saying less.
		return false, fmt.Errorf("cancelled: nothing was deployed and workload %s is as it was", workloadName)
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
	// already be locked, and locking twice answers 403. A create and a copy are
	// both drafts, so only a leftover has to be asked about.
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
func confirmRoll(live Live, workloadName string, opts Options) (bool, error) {
	// Being able to ask settles it, ahead of any inference about whether
	// anyone is there. NonInteractive is also true for --output json, which
	// says how to format stdout rather than that production may be rolled
	// without a word, and the caller passes no Confirm when the answer is
	// already in.
	if opts.Confirm != nil {
		return opts.Confirm(fmt.Sprintf(
			"Workload %s is on a locked version, which means production.%s\n"+
				"The new version will be locked too, and locking cannot be undone.\n"+
				"Type the workload name to roll it, anything else to stop: ",
			tui.WarnStyle.Render("`"+workloadName+"`"), alsoStarting(live)), workloadName)
	}

	if opts.NonInteractive {
		return true, nil
	}

	return false, errors.New(
		"this rolls a locked, production version and there is no terminal to confirm on. " +
			"Re-run with --yes to say so explicitly")
}

// alsoStarting is the clause the question needs when the workload is switched
// off, and empty when it is not.
//
// The prompt used to open "is running a locked version", which was safe while a
// stopped workload with drift was refused long before anyone was asked. It is
// asked of one now, and telling somebody their switched-off workload is running
// is exactly the wrong thing to say in the one prompt that guards production.
// The version being locked is the fact that holds either way; that the run will
// also switch the workload on is material to the answer, so it is said rather
// than left to be discovered.
func alsoStarting(live Live) string {
	if live.State != StateStopped {
		return ""
	}

	return " It is not running, and this deploy starts it."
}

// replace starts the rollout and follows it to the end. sizing is nil unless
// the runtime block changed too, in which case it travels with the swap.
//
// The candidate travels into settle as well as into the POST: the workload says
// "running" throughout a swap, so naming the artifact is what makes the wait
// that follows wait for this rollout rather than the state it was already in.
func replace(
	workloadID string,
	made version,
	lock bool,
	sizing json.RawMessage,
	result Result,
	opts Options,
	report *reporter,
) (Result, error) {
	// The guard that actually holds. The live state can have changed since
	// the one at the top, and this is the last moment before a swap that
	// cannot be taken back by refusing it.
	if err := guardRollout(workloadID,
		"the version serving keeps serving and the one just minted is left unpromoted"); err != nil {
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
		replacement, startErr := startReplacementFn(workloadID, made.ID, sizing)
		started = replacement

		return startErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot roll workload %s onto artifact %s: %w", workloadID, made.ID, err)
	}

	if opts.Detach {
		// Nobody is waiting, so the request is the whole of what this run did.
		result.Action = ActionRolled

		report.say("  Rollout of %s onto artifact %s requested; not waiting.\n", workloadID, made.ID)

		return result, nil
	}

	// Recorded after the wait rather than after the POST. A rollout that ends
	// failed never promotes and leaves the previous version serving, so a run
	// that reported "rolled" would be naming an outcome the workload did not
	// reach. What was attempted is still legible: the plan says what was
	// wanted and the artifact and build ids say what was made.
	// The two waits share one budget. Both can now run for minutes, and a
	// --poll-timeout the user set is a bound on the deploy, not on each half.
	waitFrom := time.Now()

	if err := awaitRollout(workloadID, started, opts, report); err != nil {
		return result, err
	}

	result.Action = ActionRolled

	return settle(workloadID, workload.Serving{ArtifactID: made.ID, AwaitDrain: true},
		result, budgetLeft(opts, waitFrom), report)
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
