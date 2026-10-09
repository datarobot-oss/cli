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

// PromoteWorkload locks the draft artifact the workload is running, in place,
// and returns the workload. The platform answers 422 when the artifact is
// already locked, when a replacement is in flight, or when the workload has
// no running generation, and 404 when the caller does not own the artifact.
func PromoteWorkload(workloadID string) (*Workload, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/promote")
	if err != nil {
		return nil, err
	}

	var w Workload

	if err := drapi.PostJSON(url, "workload promote request", map[string]any{}, &w); err != nil {
		return nil, err
	}

	return &w, nil
}
