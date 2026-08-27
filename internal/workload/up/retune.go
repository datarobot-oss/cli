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
	"fmt"

	"github.com/datarobot/cli/internal/workload"
)

// retune applies a change that moved only the sizing: a replica count, a
// resource allocation, a bundle, an autoscaling policy. The file says nothing
// new about what the workload runs, so no version is minted, which is the
// whole reason this is not a roll: a resize should not cost a build and a new
// immutable artifact.
//
// It is still a rollout. The platform applies new sizing by replacing the
// workload with itself, on the artifact it is already running, so this call
// starts a replacement and hands one back. Waiting on the workload alone would
// prove nothing: its status stays running for the whole swap, so the wait
// returns at once and reports a resize that may still fail as done. The
// replacement is what has to be followed, and the settle afterwards is what
// says the containers came back.
//
// The whole runtime block is sent rather than the fields that differ. It is
// the same block the plan was computed from, the platform takes it as a unit,
// and sending a subset would leave the reader guessing which half of the file
// is now live.
//
// Being a rollout is also why it is guarded. The replacement route is
// documented to queue a second swap rather than refuse one; that the settings
// route does the same is an inference from it answering with a replacement.
// A staging run confirmed the half that could be confirmed without provoking
// the bug: a settings-initiated replacement is published on the replacement
// route, with a candidate artifact id, so the guard reads what the resize
// writes. What the route does with a second PATCH while one is in flight was
// not checked, because the guard is what stops it happening. Guarding is the
// safer of the two guesses: if the route turns out to refuse on its own, the
// cost is a request and a message less specific than the platform's, while not
// guarding costs a swap nobody asked for.
//
// One guard rather than a roll's two, because nothing is minted in between, so
// the check and the call are the same moment. It goes after the payload is
// built, since that is local and free.
//
// The window it cannot close: the settings route answers 202, so a resize
// started and not waited for may not be readable on the replacement route yet
// when the next run looks. Two detached resizes back to back can still queue.
func retune(loaded Loaded, result Result, opts Options, report *reporter) (Result, error) {
	sizing, err := loaded.Compiled.RuntimePayload()
	if err != nil {
		return result, err
	}

	if err := guardRollout(result.WorkloadID, "its runtime settings were left alone"); err != nil {
		return result, err
	}

	var started *workload.Replacement

	err = report.run("Updating the runtime settings", func() error {
		replacement, updateErr := updateSettingsFn(result.WorkloadID, sizing)
		started = replacement

		return updateErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot update the runtime settings of workload %s: %w", result.WorkloadID, err)
	}

	if opts.Detach {
		// Nobody is waiting, so the request is the whole of what this run did.
		result.Action = ActionUpdated

		report.say("  Settings update for %s requested; not waiting.\n", result.WorkloadID)

		return result, nil
	}

	// After the wait, for the reason a roll records it after its own: a
	// settings replacement that ends failed leaves the workload on the sizing
	// it had, so reporting an update would name something that did not happen.
	if err := awaitResize(result.WorkloadID, started, opts, report); err != nil {
		return result, err
	}

	result.Action = ActionUpdated

	return settle(result.WorkloadID, result, opts, report)
}

// awaitResize follows the replacement a settings change starts. A failed one
// leaves the workload on the sizing it had, so it is the difference between a
// resize that happened and one that was merely accepted.
func awaitResize(workloadID string, started *workload.Replacement, opts Options, report *reporter) error {
	var settled *workload.Replacement

	err := report.run("Waiting for the new settings", func() error {
		replacement, waitErr := waitReplacementFn(workloadID, started, opts.PollInterval, opts.PollTimeout, nil)
		settled = replacement

		return waitErr
	})
	if err == nil {
		return nil
	}

	if settled != nil && workload.IsFailedReplacementStatus(settled.Status) {
		return fmt.Errorf(
			"the settings update for workload %s ended as %s, so it is still running with the sizing it had; "+
				"check 'dr workload status %s': %w",
			workloadID, settled.Status, workloadID, err)
	}

	return err
}

// rollRuntime is the runtime block to carry with a swap, and nil when the file
// and the live workload already agree about sizing.
//
// Nil rather than the block as it stands, because a replacement carrying
// runtime is a settings change as well as a rollout: sending an unchanged
// block would turn every roll into both, and a settings field the platform
// adds later would be silently reset to whatever the file happens to imply.
func rollRuntime(loaded Loaded, plan Plan) (json.RawMessage, error) {
	if len(plan.Runtime) == 0 {
		return nil, nil
	}

	return loaded.Compiled.RuntimePayload()
}
