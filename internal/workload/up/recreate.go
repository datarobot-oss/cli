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

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// recreate performs the exit every dead-workload refusal names: delete what is
// holding the name, clear the binding, and let the run create it again.
//
// It exists because naming a remedy is not the same as being able to reach it.
// The refusals spell out 'dr workload delete <id> --dir <path>', which works,
// but it is a second command against a state the user did not choose and cannot
// deploy out of; --recreate is that command folded into the deploy that was
// already asked for.
//
// The live state it returns is what the plan is built from, which is why this
// runs before the plan rather than beside the apply. Once the workload is gone
// the run is a create, and a plan that still described a roll would be
// describing something that no longer exists.
//
// It widens to nothing else. Deleting is irreversible, so every state a deploy
// can actually act on is refused rather than quietly folded in: --recreate on a
// running workload would otherwise mean "destroy production and rebuild it",
// which is not what the flag says and not what anyone reaching for it wants.
func recreate(loaded Loaded, live Live, opts Options) (Live, error) {
	if !opts.Recreate {
		return live, nil
	}

	// Nothing to delete. A first run, or one whose binding resolved to nothing,
	// already ends in a create, so the flag is satisfied by doing nothing rather
	// than by failing a run that was about to do exactly what was asked.
	if live.State == StateUnbound || live.State == StateMissing {
		return live, nil
	}

	if live.State != StateErrored && live.State != StateTerminated {
		return live, fmt.Errorf(
			"--recreate deletes and rebuilds a workload that is errored or terminated, and workload %s is %s. "+
				"Deploy without it: a workload in that state is deployed onto rather than replaced",
			name(loaded, live), live.Status)
	}

	// A dry run has to stay one, and has to stay honest. This is the only
	// mutation that happens before the plan is printed, so the delete is
	// skipped — but the state handed back is still the one the real run would
	// reach, because the plan printed here is the whole answer to "what would
	// this deploy".
	//
	// Returning the dead workload instead described a deploy that cannot
	// happen: the plan rendered a roll onto something the next line refuses,
	// and the envelope reported action "rolled" for a run whose real action is
	// "created". Nothing mutates on the strength of this, because Run stops at
	// its own DryRun branch before anything is applied.
	if opts.DryRun {
		fmt.Fprintln(opts.Stderr, tui.HintStyle.Render(fmt.Sprintf(
			"--recreate would delete workload %s (%s) and create it again under the same name. "+
				"The plan below is the one that would run after it.",
			live.WorkloadID, live.Status)))

		return Live{State: StateUnbound}, nil
	}

	agreed, err := confirmRecreate(loaded, live, opts)
	if err != nil || !agreed {
		return live, err
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	if err := report.run("Deleting the "+live.Status+" workload", func() error {
		return deleteWorkloadFn(live.WorkloadID)
	}); err != nil {
		return live, fmt.Errorf("cannot delete workload %s, so it still holds its name: %w", live.WorkloadID, err)
	}

	clearBinding(loaded, live, report)

	// Unbound rather than missing: the binding is gone from the file, not merely
	// pointing at something that answers 404, and the render each state produces
	// is the difference between "will be created" and "is bound to an id that no
	// longer exists".
	return Live{State: StateUnbound}, nil
}

// clearBinding takes the workloadId out of the manifest, and reports rather
// than fails when it cannot.
//
// The delete has already happened by this point, so failing here would stop a
// run whose irreversible half is done, over a line the create is about to
// overwrite anyway: WriteWorkloadID replaces the key in place. A clear that
// fails and a create that then fails leaves the file bound to a deleted
// workload, which the next run reads as drift and recreates — the state this
// flag exists to get out of, not a new one.
func clearBinding(loaded Loaded, live Live, report *reporter) {
	cleared, err := clearWorkloadIDFn(loaded.Path, live.WorkloadID)

	switch {
	case err != nil:
		report.say("  Could not remove %s from %s: %v\n", "workloadId", loaded.Path, err)
	case cleared:
		report.say("  Removed the binding to %s from %s.\n", live.WorkloadID, manifest.FileName)
	}
}

// confirmRecreate asks before the one irreversible thing this flag does.
//
// Typing the name back is the gesture confirmLock already uses for rolling a
// locked production version, and for the same reason: a y/n on a destructive
// question is answered by reflex, and the workload being deleted is the one
// piece of information that makes the answer considered.
//
// The three-way shape mirrors confirmRoll's. Being able to ask settles it;
// --yes is a caller who has already said so; and no terminal with no --yes is
// the case that must not proceed silently.
//
// Where it deliberately parts from confirmRoll is the middle branch, which
// reads Yes rather than NonInteractive. NonInteractive is also true for a run
// whose stdin is a pipe, so keying on it here meant
// `dr workload up --recreate < /dev/null` deleted the workload with no prompt
// and no --yes, and made the refusal below unreachable from the command.
// `dr workload delete` settles the same question the same way, and the flag
// help, the docs and the changelog all promise this one asks unless --yes.
// Rolling a locked version can be waved through by a reviewed pipeline;
// deleting cannot, because there is nothing on the other side of it.
func confirmRecreate(loaded Loaded, live Live, opts Options) (bool, error) {
	workloadName := name(loaded, live)

	if opts.Confirm != nil {
		return opts.Confirm(fmt.Sprintf(
			"Workload %s is %s. It will be deleted, which cannot be undone, and created again "+
				"under the same name.\nType the workload name to recreate it, anything else to stop: ",
			tui.WarnStyle.Render("`"+workloadName+"`"), live.Status), workloadName)
	}

	if opts.Yes {
		return true, nil
	}

	return false, errors.New(
		"--recreate deletes a workload, which cannot be undone, and there is no terminal to confirm on. " +
			"Re-run with --yes to say so explicitly")
}
