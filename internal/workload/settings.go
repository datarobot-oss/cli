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
// 200 having changed nothing, which is indistinguishable from success.
//
// Known limitation. Unlike the rest of this package, this route has not been
// exercised against a live instance; it is what the platform's own workload
// documentation describes. If it turns out to want the runtime fields at the
// top level rather than under a runtime key, or a different path segment, this
// function is the only place that has to change.
func UpdateWorkloadSettings(workloadID string, runtime json.RawMessage) (*Workload, error) {
	if len(runtime) == 0 {
		return nil, errors.New("no runtime settings to update")
	}

	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/settings/")
	if err != nil {
		return nil, err
	}

	body := map[string]json.RawMessage{"runtime": runtime}

	var updated Workload

	if err := drapi.PatchJSON(url, "workload settings", body, &updated); err != nil {
		return nil, err
	}

	return &updated, nil
}
