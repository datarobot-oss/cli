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

package workload

import (
	"encoding/json"
	"errors"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// UpdateWorkloadSettings changes how much a workload runs with, leaving what
// it runs alone: replica counts, resource allocations, resource bundles, an
// autoscaling policy. It is the one kind of change a deploy can make in place,
// because it says nothing about the image or the code, and so needs no new
// artifact and no rollout.
//
// runtime is the manifest's runtime block, sent as it stands. Passing an empty
// one is refused rather than sent: a PATCH with nothing in it would come back
// having changed nothing, which is indistinguishable from success.
//
// What comes back is a Replacement, not a workload, and the status is 202: the
// platform carries out a settings change by rolling the workload onto the
// artifact it is already running with the new sizing applied. So this call
// only starts the work, and the caller has to follow the replacement to know
// whether the new sizing was promoted. The workload's own status is no help,
// since it stays running throughout.
func UpdateWorkloadSettings(workloadID string, runtime json.RawMessage) (*Replacement, error) {
	if len(runtime) == 0 {
		return nil, errors.New("no runtime settings to update")
	}

	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/settings/")
	if err != nil {
		return nil, err
	}

	body := map[string]json.RawMessage{"runtime": runtime}

	var started Replacement

	if err := drapi.PatchJSON(url, "workload settings", body, &started); err != nil {
		return nil, err
	}

	return &started, nil
}
