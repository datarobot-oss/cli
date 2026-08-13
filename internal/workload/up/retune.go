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
// new about what the workload runs, so no version is minted and nothing is
// swapped. It is one call and a wait, which is the whole reason this is not a
// roll: a resize should not cost a build, a new immutable artifact and a
// rollout.
//
// The whole runtime block is sent rather than the fields that differ. It is
// the same block the plan was computed from, the platform takes it as a unit,
// and sending a subset would leave the reader guessing which half of the file
// is now live.
func retune(loaded Loaded, result Result, opts Options, report *reporter) (Result, error) {
	runtime, err := loaded.Compiled.RuntimePayload()
	if err != nil {
		return result, err
	}

	var updated *workload.Workload

	err = report.run("Updating the runtime settings", func() error {
		wl, updateErr := updateSettingsFn(result.WorkloadID, runtime)
		updated = wl

		return updateErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot update the runtime settings of workload %s: %w", result.WorkloadID, err)
	}

	result.Action = ActionUpdated

	if updated != nil {
		result.Status = updated.Status
	}

	if opts.Detach {
		report.say("  Settings update for %s requested; not waiting.\n", result.WorkloadID)

		return result, nil
	}

	// The wait is the same one every other path ends with. A settings change
	// can restart containers, so reporting success the moment the platform
	// accepts it would name a workload that is on its way down.
	return settle(result.WorkloadID, result, opts, report)
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
