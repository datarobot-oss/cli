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
	"strings"

	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// The Use Case link seams, beside the others in run.go for the same reason.
var (
	workloadUseCasesFn = workload.WorkloadUseCases
	linkUseCaseFn      = workload.LinkWorkloadToUseCase
	useCaseEnclavesFn  = workload.UseCaseEnclaveCount
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
//
// A link is only ever added. Other Use Cases the workload is in stay: up has
// no record of which links it made, so removing the rest would also remove
// links made in the UI or by a teammate, and the access that came with them.
//
// A deploy that cannot read what it needs stops here, before anything is
// changed: the file asks for the link, and a run that cannot make sure of it
// does not apply the rest without it. A dry run changes nothing either way,
// so it previews the link as missing and says why it could not tell.
func planUseCase(plan Plan, workloadID, desired string, payload json.RawMessage, dryRun bool) (*UseCasePlan, error) {
	if desired == "" {
		return nil, nil //nolint:nilnil // no Use Case asked for is not an error
	}

	if plan.Creates || workloadID == "" {
		return &UseCasePlan{ID: desired}, nil
	}

	links, err := workloadUseCasesFn(workloadID)
	if err != nil {
		err = fmt.Errorf("cannot read which Use Cases workload %s is linked to: %w", workloadID, err)

		return unknownUseCase(desired, err, dryRun)
	}

	out := &UseCasePlan{ID: desired, Linked: links.Has(desired)}

	if !out.Linked {
		if err := checkEnclaves(workloadID, desired, payload); err != nil {
			if errors.Is(err, errEnclaveRead) {
				return unknownUseCase(desired, err, dryRun)
			}

			return nil, err
		}
	}

	return out, nil
}

// unknownUseCase is a read that failed: the error on a deploy, and on a dry
// run a link previewed as missing, with the reason.
func unknownUseCase(desired string, err error, dryRun bool) (*UseCasePlan, error) {
	if !dryRun {
		return nil, err
	}

	return &UseCasePlan{ID: desired, Unknown: err.Error()}, nil
}

// errEnclaveRead marks a failed read of the Use Case's Enclaves, which a dry
// run previews rather than fails on.
var errEnclaveRead = errors.New("cannot read the Use Case")

// checkEnclaves keeps create's placement rule when an existing workload is
// linked. The platform refuses to create a workload in a Use Case with
// Enclaves unless it names a selection policy, because accepting it would put
// it on the Hub without a word; the link route makes no such check, so the
// run makes it before linking anything.
func checkEnclaves(workloadID, useCaseID string, payload json.RawMessage) error {
	policy, err := enclavePolicy(payload)
	if err != nil || policy != "" {
		return err
	}

	count, err := useCaseEnclavesFn(useCaseID)
	if err != nil {
		return fmt.Errorf("%w %s to check its Enclaves: %w", errEnclaveRead, useCaseID, err)
	}

	if count == 0 {
		return nil
	}

	return enclaveRefusal(fmt.Errorf("nothing was deployed: workload %s cannot be linked to Use Case %s (%s)",
		workloadID, useCaseID, enclaveTargetingRequired))
}

// enclavePolicy is runtime.enclaveSelectionPolicy in the compiled payload,
// "" when the file sets none.
func enclavePolicy(payload json.RawMessage) (string, error) {
	var doc struct {
		Runtime struct {
			Policy *string `json:"enclaveSelectionPolicy"`
		} `json:"runtime"`
	}

	if err := json.Unmarshal(payload, &doc); err != nil {
		return "", fmt.Errorf("cannot read the compiled manifest: %w", err)
	}

	if doc.Runtime.Policy == nil {
		return "", nil
	}

	return strings.TrimSpace(*doc.Runtime.Policy), nil
}

// link adds the link the plan asks for, and reports whether it sent one. It
// runs before anything is started or rolled, because the platform checks a
// rollout against the workload's links.
func link(plan Plan, workloadID string, report *reporter) (bool, error) {
	if !plan.relinks() {
		return false, nil
	}

	uc := plan.UseCase

	err := report.run("Linking to Use Case "+uc.ID, func() error {
		return linkUseCaseFn(uc.ID, workloadID)
	})
	if err != nil {
		return false, fmt.Errorf("nothing was deployed: workload %s could not be linked to Use Case %s: %w",
			workloadID, uc.ID, err)
	}

	return true, nil
}

// enclaveRefusal adds the remedy to a refusal of a workload whose Use Case
// has Enclaves and which names no selection policy: the platform's, on a
// create, or checkEnclaves', on a link.
func enclaveRefusal(err error) error {
	if err == nil || !strings.Contains(err.Error(), enclaveTargetingRequired) {
		return err
	}

	return fmt.Errorf("%w. The Use Case has Enclaves, so the workload has to say how it is placed on them: "+
		"set runtime.enclaveSelectionPolicy: availability in %s to let DataRobot pick one, then deploy again",
		err, manifest.FileName)
}
