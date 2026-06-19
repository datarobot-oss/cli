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
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// WorkloadOperationResponse is the acknowledgement from POST
// /workloads/{id}/start and /stop. Status is a human-readable message, not a
// status enum; TrackVia is the path to poll for the transition.
type WorkloadOperationResponse struct {
	Status     string `json:"status"`
	WorkloadID string `json:"workloadId"`
	TrackVia   string `json:"trackVia"`
}

// StartWorkload requests an asynchronous start of a stopped workload. 202
// queues it, 200 is an idempotent no-op (already running/initializing); 409
// (must stop first), 403 (limits), and 404 surface as *drapi.HTTPError.
func StartWorkload(workloadID string) (*WorkloadOperationResponse, error) {
	return postWorkloadAction(workloadID, "start")
}

// StopWorkload requests an asynchronous stop of a workload. 202 queues it,
// 200 is an idempotent no-op (already stopped); 404 surfaces as
// *drapi.HTTPError.
func StopWorkload(workloadID string) (*WorkloadOperationResponse, error) {
	return postWorkloadAction(workloadID, "stop")
}

// postWorkloadAction POSTs an empty JSON object to a workload action
// sub-path (no trailing slash, unlike GetWorkload's resource route).
func postWorkloadAction(workloadID, action string) (*WorkloadOperationResponse, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/" + action)
	if err != nil {
		return nil, err
	}

	var resp WorkloadOperationResponse

	if err := drapi.PostJSON(url, "workload "+action+" request", map[string]any{}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
