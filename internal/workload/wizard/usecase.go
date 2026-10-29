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

package wizard

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// linkUseCase writes the Use Case the workload is linked to into rendered
// content, and returns it. The Workload API needs one to place a workload on
// an Enclave, and refuses to restart or replace a workload already on one
// without it.
//
// The choice, in order: a useCaseId the content already carries, the link a
// bound workload already has (it cannot be changed through the Workload API),
// --use-case-id or the --spec-file's useCaseId, and otherwise the most
// recently updated Use Case. A new workload whose Use Case has Enclaves also gets the
// availability policy, without which the create is refused.
//
// Not finding a Use Case to pick is not an error: a workload off any Enclave
// needs none, so the file then names none.
func (o Options) linkUseCase(content []byte) ([]byte, string, error) {
	parsed, err := manifest.Parse(content, "")
	if err != nil {
		// checkRendered reports it, in the words that fit its author.
		return content, "", nil
	}

	asked, err := o.askedUseCase()
	if err != nil {
		return nil, "", err
	}

	// Only a hand edit on the confirm screen puts one there.
	if named := parsed.UseCaseID(); named != "" {
		return content, named, nil
	}

	bound := parsed.WorkloadID()

	if bound != "" {
		linked, err := workloadUseCaseFn(bound)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read the Use Case of workload %s: %w", bound, err)
		}

		if linked != nil {
			if asked != "" && asked != linked.ID {
				return nil, "", fmt.Errorf("workload %s is linked to Use Case %s, and a workload's Use Case "+
					"cannot be changed: drop --use-case-id, or create a new workload with --name", bound, linked.ID)
			}

			o.sayf("Use Case: %s (%s), the one workload %s is linked to.\n", linked.Name, linked.ID, bound)

			return setUseCase(content, linked.ID, false)
		}
	}

	return o.pickUseCase(content, asked, bound == "")
}

// pickUseCase links content to --use-case-id, or to the most recently updated
// Use Case when none was asked for. Only a new workload is checked for
// Enclaves: a bound one keeps the placement it already runs with.
func (o Options) pickUseCase(content []byte, asked string, isNew bool) ([]byte, string, error) {
	id, label := asked, asked

	if id == "" {
		latest, err := latestUseCaseFn()
		if err != nil {
			o.sayf("Could not list Use Cases (%v), so the manifest names none. "+
				"Pass --use-case-id to link one.\n", err)

			return content, "", nil
		}

		if latest == nil {
			return content, "", nil
		}

		id = latest.ID
		label = fmt.Sprintf("%s (%s), the most recently updated. Pass --use-case-id to link another", latest.Name, id)
	}

	onEnclave := false

	if isNew {
		var err error

		onEnclave, err = useCaseEnclavesFn(usecase.ID(id))
		if err != nil {
			return nil, "", fmt.Errorf("cannot read the Enclaves of Use Case %s: %w", id, err)
		}
	}

	o.sayf("Use Case: %s.\n", label)

	if onEnclave {
		o.sayf("  It has Enclaves, so the workload is placed on one of them " +
			"(runtime.enclaveSelectionPolicy: availability).\n")
	}

	return setUseCase(content, id, onEnclave)
}

// askedUseCase is the Use Case the run was told to use: --use-case-id, or the
// useCaseId of a --spec-file, which the rendered manifest does not carry over.
func (o Options) askedUseCase() (string, error) {
	flag, spec := strings.TrimSpace(o.Answers.UseCaseID), ""

	if o.SpecFile != "" {
		if raw, err := workload.ReadSpecFile(o.SpecFile); err == nil {
			var doc struct {
				UseCaseID string `json:"useCaseId"`
			}

			// A useCaseId of another type stays "", as if absent.
			_ = json.Unmarshal(raw, &doc)
			spec = strings.TrimSpace(doc.UseCaseID)
		}
	}

	if flag != "" && spec != "" && flag != spec {
		return "", fmt.Errorf("--use-case-id %s disagrees with useCaseId %s in %s: drop one", flag, spec, o.SpecFile)
	}

	if flag == "" && spec == "" {
		return "", nil
	}

	id, err := usecase.ParseID(cmp.Or(flag, spec))
	if err != nil {
		return "", fmt.Errorf("invalid Use Case id: %w", err)
	}

	return string(id), nil
}

func setUseCase(content []byte, id string, onEnclave bool) ([]byte, string, error) {
	updated, err := manifest.SetUseCase(content, id, onEnclave)
	if err != nil {
		return nil, "", err
	}

	return updated, id, nil
}

func (o Options) sayf(format string, args ...any) {
	if !o.quiet() {
		fmt.Fprintf(o.Stderr, format, args...)
	}
}
