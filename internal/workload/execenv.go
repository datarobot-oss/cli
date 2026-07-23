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
	"fmt"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// ExecutionEnvironment is the projection of the server's EE document the CLI
// needs to build a generated-Dockerfile artifact spec.
type ExecutionEnvironment struct {
	ID                      string     `json:"id"`
	Name                    string     `json:"name"`
	LatestSuccessfulVersion *EEVersion `json:"latestSuccessfulVersion"`
}

// EEVersion is an execution environment version reference.
type EEVersion struct {
	ID string `json:"id"`
}

type executionEnvironmentList struct {
	Data []ExecutionEnvironment `json:"data"`
	Next string                 `json:"next"`
}

// ResolveExecutionEnvironment finds an execution environment by exact id or
// name and returns its id and latest successful version id. `dr workload up`
// uses it to fill the executionEnvironmentId/versionId of a generated-Dockerfile
// artifact so the user names an EE without pasting ids.
func ResolveExecutionEnvironment(nameOrID string) (id, versionID string, err error) {
	pageURL, err := config.GetEndpointURL("/api/v2/executionEnvironments/?limit=100")
	if err != nil {
		return "", "", err
	}

	for pageURL != "" {
		var list executionEnvironmentList

		if err := drapi.GetJSON(pageURL, "execution environments", &list); err != nil {
			return "", "", err
		}

		for _, ee := range list.Data {
			if ee.ID != nameOrID && ee.Name != nameOrID {
				continue
			}

			if ee.LatestSuccessfulVersion == nil {
				return "", "", fmt.Errorf("execution environment %q has no successful version to build from", nameOrID)
			}

			return ee.ID, ee.LatestSuccessfulVersion.ID, nil
		}

		if list.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return "", "", err
		}

		pageURL = list.Next
	}

	return "", "", fmt.Errorf("execution environment %q not found; list options with `dr` against /executionEnvironments/", nameOrID)
}
