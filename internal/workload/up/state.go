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

import "github.com/datarobot/cli/internal/workload"

// State is the live workload reduced to the cases that change what `up` does
// next. The platform has eleven statuses; only these distinctions matter to a
// deploy, and collapsing them here keeps the branch table in one place
// instead of spread across the apply path.
type State int

const (
	// StateUnbound is a manifest with no workloadId. The first `up` creates
	// the workload and writes the id back.
	StateUnbound State = iota

	// StateMissing is a manifest naming a workload the platform does not
	// have, usually because it was deleted from the UI or with
	// `dr workload delete`. It is drift like any other: the file says what
	// should exist, so `up` creates it and the plan names the id that went
	// missing.
	//
	// This reverses an earlier rule that refused and told the user to rebind
	// or clear the id by hand. The reasoning behind that rule does not hold:
	// it feared leaving two workloads of the same name with no way to tell
	// which one the URL pointed at, but a 404 means the first one is gone, so
	// there is no pair. What refusing did produce was a dead end, on the
	// recovery path, pointing at a `config --workload-id` that will not edit
	// an existing manifest.
	//
	// The hazard that is real: a 404 also means "not on this instance". A
	// manifest deployed against the wrong endpoint, organisation or token
	// resolves to nothing and is recreated somewhere unintended. Refusing
	// everyone to catch that case was the wrong trade, so the plan names the
	// missing id instead. That line is the whole warning, because the plan is
	// announced and applied without a confirm.
	StateMissing

	// StateTerminated is the end of a workload's life and is not restartable.
	// It is not treated like StateMissing, though the two look alike from the
	// file's side. A terminated workload still exists: the platform returns
	// it, and it keeps its name and its artifact, so creating over it collides
	// with the name it still owns. `up` refuses and names the one thing that
	// clears the way, which is deleting it.
	StateTerminated

	// StateStopped covers stopped, suspended and interrupted. `up` starts it
	// and then reconciles, since the user asking to deploy is asking for it
	// to be running.
	StateStopped

	// StateSettling is a workload still moving under its own power. `up`
	// waits for it rather than acting on a state that is about to change.
	StateSettling

	// StateRunning is the ordinary case: reconcile against it.
	StateRunning

	// StateErrored is a workload the platform could not bring up. `up` prints
	// the plan, refuses to roll onto it and points at the logs. It does not
	// guess, because a slow start can report errored and then recover.
	StateErrored
)

// String names the state for a plan line or an error message.
func (s State) String() string {
	switch s {
	case StateUnbound:
		return "not created yet"
	case StateMissing:
		return "missing"
	case StateTerminated:
		return workload.WorkloadStatusTerminated
	case StateStopped:
		return "stopped"
	case StateSettling:
		return "settling"
	case StateRunning:
		return workload.WorkloadStatusRunning
	case StateErrored:
		return workload.WorkloadStatusErrored
	default:
		return "unknown"
	}
}

// stateFor maps a platform status onto the branch `up` takes.
//
// Two of these are judgement calls rather than transcription. "unknown" is
// treated as settling rather than as an error, because it is what the server
// reports before it has decided anything and erroring on it would fail
// deploys that were merely early. "interrupted" is grouped with stopped
// rather than with errored, because it is a workload the platform took down
// and will let you start again, not one that broke.
func stateFor(status string) State {
	switch status {
	case workload.WorkloadStatusRunning:
		return StateRunning

	case workload.WorkloadStatusStopped,
		workload.WorkloadStatusSuspended,
		workload.WorkloadStatusInterrupted:
		return StateStopped

	case workload.WorkloadStatusTerminated:
		return StateTerminated

	case workload.WorkloadStatusErrored:
		return StateErrored

	default:
		// submitted, provisioning, launching, stopping, unknown, and
		// anything the platform adds later. Waiting is always safe; guessing
		// is not.
		return StateSettling
	}
}
