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
	"time"

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
	waitFrom := time.Now()

	if err := awaitResize(result.WorkloadID, started, opts, report, resizeWait{
		label:   "Waiting for the new settings",
		act:     "the settings update for",
		outcome: "it is still running with the sizing it had",
	}); err != nil {
		return result, err
	}

	result.Action = ActionUpdated

	// A resize changes no artifact, but it does replace a generation, so the
	// wait still has to see the outgoing one stop answering.
	return settle(result.WorkloadID, workload.Serving{AwaitDrain: true},
		result, budgetLeft(opts, waitFrom), report)
}

// awaitResize follows the replacement a settings change starts. A failed one
// leaves the workload on the sizing it had, so it is the difference between a
// resize that happened and one that was merely accepted.
func awaitResize(
	workloadID string, started *workload.Replacement, opts Options, report *reporter, wait resizeWait,
) error {
	var settled *workload.Replacement

	err := report.run(wait.label, func() error {
		replacement, waitErr := waitReplacementFn(workloadID, started, opts.PollInterval, opts.PollTimeout, nil)
		settled = replacement

		return waitErr
	})
	if err == nil {
		return nil
	}

	if settled != nil && workload.IsFailedReplacementStatus(settled.Status) {
		return fmt.Errorf(
			"%s workload %s ended as %s, so %s; check 'dr workload status %s': %w",
			wait.act, workloadID, settled.Status, wait.outcome, workloadID, err)
	}

	return err
}

// resizeWait says what a settings replacement was asked for, so the line the
// user watches and the sentence a failure produces describe the run they asked
// for rather than the mechanism it borrows. The platform carries out every
// settings change by rolling the workload onto the artifact it is already
// running, which makes the same call a resize and a restart.
type resizeWait struct {
	// label is what the spinner says while the swap is in flight.
	label string
	// act names the operation and carries its preposition, completing
	// "<act> workload <id> ended as <status>".
	act string
	// outcome says what the workload is left doing, completing "so <outcome>".
	outcome string
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

// notRestarted is what a refusal between the rotation and the restart has to
// say. The store has already taken the new value, the workload has not, and
// the error is the only account of that the run gives: a bare refusal would
// read as a run that did nothing, and the next bare run calls the workload up
// to date while it serves the old value.
func notRestarted(result Result, err error) error {
	return fmt.Errorf(
		"%d %s re-sent to the credential store, but workload %s was not restarted to pick %s up, "+
			"so the new value is not being served yet: %w",
		result.Env.SecretsRotated,
		plural(result.Env.SecretsRotated, "secret was", "secrets were"),
		result.WorkloadID, plural(result.Env.SecretsRotated, "it", "them"), err)
}

// restartRuntime is the runtime block a restart sends: the file's, so no
// sizing moves, and the live workload's when the file names none.
//
// The settings PATCH is the restart, and it has to carry a block. A manifest
// without one leaves sizing to the platform, which is a shape a deploy accepts
// and a resize never reaches, since there is nothing in the file to have
// drifted; a rotation-only run reaches it, and what the platform settled on is
// what the workload already runs with, so sending it back moves nothing.
func restartRuntime(loaded Loaded, live Live) (json.RawMessage, error) {
	runtime, err := loaded.Runtime()
	if err != nil {
		return nil, err
	}

	if len(runtime) == 0 {
		runtime = live.Runtime
	}

	if len(runtime) == 0 {
		return nil, fmt.Errorf("neither the manifest nor workload %s carries a runtime block, and a restart "+
			"has to send one", live.WorkloadID)
	}

	raw, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the runtime block to JSON: %w", err)
	}

	return raw, nil
}

// restartForRotation replaces the workload with itself, so a secret this run
// re-sent to the credential store is the one actually being served.
//
// A rotation writes to the store and to no file, so a run with nothing else to
// do produces a plan the deploy calls empty and replaces no container. A
// container reads its credentials when it starts, so without this the new
// value sits in the store while the workload goes on serving the old one, and
// the run reports itself up to date. It is, about the manifest. It is not
// about the thing taking requests, which is what the person who rotated a key
// was asking about.
//
// The settings PATCH is the restart. The platform carries every settings
// change out by rolling the workload onto the artifact it is already running,
// so this is the same rolling swap a resize gets: a new generation comes up,
// the old one drains, and the endpoint answers throughout. The runtime sent is
// the one the file already asks for, or the one the workload already runs with
// when the file leaves sizing to the platform, so no sizing moves; what the
// call buys is the new generation.
//
// The alternative is the stop and start this used to print for the user to run
// by hand. That is a real outage, on a workload they asked only to reconcile,
// through two commands they should not have to know about.
func restartForRotation(loaded Loaded, live Live, result Result, opts Options) (Result, error) {
	if result.Env.SecretsRotated == 0 {
		return result, nil
	}

	// A workload that cannot take a deploy cannot take a restart either, and
	// the refusal names what to do about the state it is in. Every refusal
	// from here to the PATCH is wrapped the same way, because the store has
	// already moved by now, and a bare refusal reads as a run that did nothing.
	if err := deployable(live, result.Name, dirFlagFor(loaded)); err != nil {
		return result, notRestarted(result, err)
	}

	sizing, err := restartRuntime(loaded, live)
	if err != nil {
		return result, notRestarted(result, err)
	}

	if err := guardRollout(result.WorkloadID,
		"the re-sent secret is in the credential store and not yet serving"); err != nil {
		return result, notRestarted(result, err)
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	var started *workload.Replacement

	err = report.run("Restarting to pick up the re-sent secret", func() error {
		replacement, updateErr := updateSettingsFn(result.WorkloadID, sizing)
		started = replacement

		return updateErr
	})
	if err != nil {
		return result, fmt.Errorf(
			"%d %s re-sent to the credential store, but workload %s could not be restarted to pick %s up, "+
				"so it is still serving the value it had: %w",
			result.Env.SecretsRotated,
			plural(result.Env.SecretsRotated, "secret was", "secrets were"),
			result.WorkloadID, plural(result.Env.SecretsRotated, "it", "them"), err)
	}

	// Recorded before the wait, unlike a resize, because the two report
	// different things. A resize that fails leaves the sizing it had, so
	// claiming an update would name something that did not happen; a restart
	// that fails has still moved the credential store, and the run has to say
	// it did something or the next bare run will call the workload up to date
	// while it serves the old value.
	result.Action = ActionUpdated

	if opts.Detach {
		report.say("  Restart of %s requested; not waiting.\n", result.WorkloadID)

		return result, nil
	}

	waitFrom := phaseClock()

	if err := awaitResize(result.WorkloadID, started, opts, report, resizeWait{
		label:   "Waiting for the restart",
		act:     "the restart of",
		outcome: "it is still serving the value it had",
	}); err != nil {
		return result, err
	}

	// The same drain a resize waits for, and for a sharper reason. The
	// replacement completing says the new generation came up; it does not say
	// the old one stopped answering, and the old one is the one still serving
	// the value this run replaced. Returning at the line above would report a
	// rotation as landed while the key it replaced was still being handed out.
	// It is also what puts a container that crashes on the new value in front
	// of the reader, instead of exit 0 and a workload that is not running.
	return settle(result.WorkloadID, workload.Serving{AwaitDrain: true, Restart: true},
		result, budgetLeft(opts, waitFrom), report)
}
