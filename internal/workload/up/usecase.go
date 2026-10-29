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
	"fmt"

	"github.com/datarobot/cli/internal/usecase"
)

// linkUseCase links a workload that already exists to the Use Case its
// manifest names. A create sends useCaseId in its payload, but the Workload
// API takes it on create only, so a workload made before the file named one,
// or unlinked since, is linked through the Use Case API instead.
//
// A workload already linked elsewhere is left there, because its Use Case
// cannot be changed; the run says so rather than failing over it.
func linkUseCase(loaded Loaded, live Live, opts Options) error {
	workloadID := boundID(live)
	named := loaded.Manifest.UseCaseID()

	if workloadID == "" || named == "" {
		return nil
	}

	id, err := usecase.ParseID(named)
	if err != nil {
		return fmt.Errorf("%s: invalid useCaseId: %w", loaded.Path, err)
	}

	linked, err := workloadUseCaseFn(workloadID)
	if err != nil {
		return fmt.Errorf("cannot read the Use Case of workload %s: %w", workloadID, err)
	}

	if linked != nil {
		if linked.ID != string(id) {
			fmt.Fprintf(opts.Stderr, "  Workload %s stays linked to Use Case %s, not the %s the manifest names: "+
				"a workload's Use Case cannot be changed.\n", workloadID, linked.ID, id)
		}

		return nil
	}

	if err := linkUseCaseFn(id, workloadID); err != nil {
		return fmt.Errorf("cannot link workload %s to Use Case %s: %w", workloadID, id, err)
	}

	fmt.Fprintf(opts.Stderr, "  Linked workload %s to Use Case %s.\n", workloadID, id)

	return nil
}
