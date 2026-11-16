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
	"strings"

	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// The Use Case link seams, beside the others in run.go for the same reason.
var (
	workloadUseCasesFn = workload.WorkloadUseCases
	linkUseCaseFn      = workload.LinkWorkloadToUseCase
	unlinkUseCaseFn    = workload.UnlinkWorkloadFromUseCase
)

// enclaveTargetingRequired is the code the platform refuses a workload with
// when its Use Case has Enclaves and it names no selection policy.
const enclaveTargetingRequired = "ENCLAVE_TARGETING_REQUIRED"

// withUseCase settles which Use Case the run links the workload to, and puts
// it in the create payload when the flag names one the file does not.
//
// The manifest's top-level useCaseId is the source, as every other field of
// the file is, and --use-case-id fills it in or has to agree with it. Without
// this the field was read only by the create: added to the file after the
// first deploy, it was ignored with no word. The flag is never written back
// to the file, so a deploy stays the committed repo plus the flags passed.
func withUseCase(loaded Loaded, flag string) (Loaded, string, error) {
	fromFile, err := manifestUseCase(loaded.Compiled.Payload)
	if err != nil {
		return loaded, "", err
	}

	if flag == "" {
		return loaded, fromFile, nil
	}

	id, err := usecase.ParseID(flag)
	if err != nil {
		return loaded, "", fmt.Errorf("invalid --use-case-id: %w", err)
	}

	if string(id) == fromFile {
		return loaded, fromFile, nil
	}

	if fromFile != "" {
		return loaded, "", fmt.Errorf(
			"%s already sets useCaseId to %s; remove it from the file or drop --use-case-id %s",
			manifest.FileName, fromFile, id)
	}

	payload, err := workload.ApplyUseCase(loaded.Compiled.Payload, id)
	if err != nil {
		return loaded, "", err
	}

	// A copy, so the caller's Compiled is not edited behind its back.
	compiled := *loaded.Compiled
	compiled.Payload = payload
	loaded.Compiled = &compiled

	return loaded, string(id), nil
}

// manifestUseCase is the Use Case the file names, "" for none. A blank or
// null useCaseId names none, as it does on a create.
func manifestUseCase(payload json.RawMessage) (string, error) {
	var doc map[string]any

	if err := json.Unmarshal(payload, &doc); err != nil {
		return "", fmt.Errorf("cannot read the compiled manifest: %w", err)
	}

	switch value := doc["useCaseId"].(type) {
	case nil:
		return "", nil

	case string:
		if strings.TrimSpace(value) == "" {
			return "", nil
		}

		id, err := usecase.ParseID(value)
		if err != nil {
			return "", fmt.Errorf("invalid useCaseId in %s: %w", manifest.FileName, err)
		}

		return string(id), nil

	default:
		return "", fmt.Errorf("invalid useCaseId in %s: it must be a string", manifest.FileName)
	}
}

// planUseCase reads the links of a workload that exists and works out what
// the run changes about them. A create has nothing to read: the link goes in
// its payload. The read runs on a dry run too, because without it the plan
// cannot say whether there is anything to link.
func planUseCase(plan Plan, workloadID, desired string) (*UseCasePlan, error) {
	if desired == "" {
		return nil, nil //nolint:nilnil // no Use Case asked for is not an error
	}

	if plan.Creates || workloadID == "" {
		return &UseCasePlan{ID: desired}, nil
	}

	links, err := workloadUseCasesFn(workloadID)
	if err != nil {
		return nil, fmt.Errorf("cannot read which Use Cases workload %s is linked to: %w", workloadID, err)
	}

	out := &UseCasePlan{ID: desired, Linked: links.Has(desired), Partial: links.More}

	for _, id := range links.IDs {
		if id != desired {
			out.Unlink = append(out.Unlink, id)
		}
	}

	return out, nil
}

// link adds the link the plan asks for, and reports whether it sent one. It
// runs before anything is started or rolled, because the platform checks a
// rollout against the workload's links.
func link(plan Plan, workloadID string, report *reporter) (bool, error) {
	if !plan.relinks() || plan.UseCase.Linked {
		return false, nil
	}

	uc := plan.UseCase

	err := report.run("Linking to Use Case "+uc.ID, func() error {
		return linkUseCaseFn(uc.ID, workloadID)
	})
	if err != nil {
		return false, enclaveRefusal(fmt.Errorf("nothing was deployed: workload %s could not be linked to Use Case %s: %w",
			workloadID, uc.ID, err))
	}

	return true, nil
}

// unlinkOthers takes the workload out of every other Use Case the plan read.
// It runs only once the rest of the deploy has gone through, so a run that
// fails or is interrupted halfway leaves the workload where it was as well
// as where it is going, never moved off the old Use Case with nothing new
// deployed; the next run finishes the move.
func unlinkOthers(plan Plan, workloadID string, report *reporter) error {
	if !plan.relinks() {
		return nil
	}

	uc := plan.UseCase

	for _, old := range uc.Unlink {
		err := report.run("Unlinking from Use Case "+old, func() error {
			return unlinkUseCaseFn(old, workloadID)
		})
		if err != nil {
			return fmt.Errorf("workload %s is linked to Use Case %s, but could not be unlinked from %s: %w",
				workloadID, uc.ID, old, err)
		}
	}

	return nil
}

// enclaveRefusal adds the remedy to the platform's refusal of a workload
// whose Use Case has Enclaves and which names no selection policy. The client
// cannot tell beforehand whether a Use Case has Enclaves without asking, so
// the server's refusal is the check, and this is what makes it actionable.
func enclaveRefusal(err error) error {
	if err == nil || !strings.Contains(err.Error(), enclaveTargetingRequired) {
		return err
	}

	return fmt.Errorf("%w. The Use Case has Enclaves, so the workload has to say how it is placed on them: "+
		"set runtime.enclaveSelectionPolicy: availability in %s to let DataRobot pick one, then deploy again",
		err, manifest.FileName)
}
