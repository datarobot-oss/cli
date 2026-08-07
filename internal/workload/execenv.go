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
	"strings"

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

// maxExecEnvPages bounds the paging walk. The list has no natural stopping
// point the way the limit-bounded listings do, because it scans for a match
// rather than collecting a page's worth, so a server that keeps handing back a
// "next" cursor would page forever. Hitting the cap is a broken server, not a
// large tenant: 100 pages of 100 is well past any real environment count.
const maxExecEnvPages = 100

// ResolveExecutionEnvironment finds an execution environment by exact id or
// name and returns its id and latest successful version id. `dr workload up`
// uses it to fill the executionEnvironmentId/versionId of a generated-Dockerfile
// artifact so the user names an EE without pasting ids.
//
// An id match wins immediately, since ids are unique. Names are not: the
// platform catalog and a tenant's own environments can carry the same one, so
// name matches are collected across every page and an ambiguous name is an
// error rather than whichever copy the server happened to list first. Picking
// silently would build against the wrong base image and only show up as a
// puzzling runtime failure.
func ResolveExecutionEnvironment(nameOrID string) (id, versionID string, err error) {
	byID, byName, err := scanExecutionEnvironments(nameOrID)
	if err != nil {
		return "", "", err
	}

	if byID != nil {
		return resolveVersion(*byID, nameOrID)
	}

	if len(byName) > 1 {
		return "", "", fmt.Errorf(
			"execution environment %q is ambiguous: %d environments share that name (%s). Pass the id instead",
			nameOrID, len(byName), strings.Join(execEnvIDs(byName), ", "),
		)
	}

	if len(byName) == 1 {
		return resolveVersion(byName[0], nameOrID)
	}

	return "", "", fmt.Errorf("execution environment %q not found; check the name in the DataRobot UI under Registry > Environments", nameOrID)
}

// scanExecutionEnvironments walks the paged listing once, returning the unique
// id match if one turned up and every environment carrying the name. It stops
// early on an id hit; a name has to be checked against the whole list before it
// can be called unambiguous.
func scanExecutionEnvironments(nameOrID string) (byID *ExecutionEnvironment, byName []ExecutionEnvironment, err error) {
	pageURL, err := config.GetEndpointURL("/api/v2/executionEnvironments/?limit=100")
	if err != nil {
		return nil, nil, err
	}

	for pages := 0; pageURL != ""; pages++ {
		if pages >= maxExecEnvPages {
			return nil, nil, fmt.Errorf("execution environment %q not found after %d pages of results", nameOrID, maxExecEnvPages)
		}

		var list executionEnvironmentList

		if err := drapi.GetJSON(pageURL, "execution environments", &list); err != nil {
			return nil, nil, err
		}

		for _, ee := range list.Data {
			if ee.ID == nameOrID {
				return &ee, nil, nil
			}

			if ee.Name == nameOrID {
				byName = append(byName, ee)
			}
		}

		if list.Next == "" {
			return nil, byName, nil
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return nil, nil, err
		}

		pageURL = list.Next
	}

	return nil, byName, nil
}

// resolveVersion unwraps the version a build can actually target. nameOrID is
// carried through so the error names what the user typed rather than whichever
// of the id or name matched.
func resolveVersion(ee ExecutionEnvironment, nameOrID string) (id, versionID string, err error) {
	if ee.LatestSuccessfulVersion == nil {
		return "", "", fmt.Errorf("execution environment %q has no successful version to build from", nameOrID)
	}

	return ee.ID, ee.LatestSuccessfulVersion.ID, nil
}

func execEnvIDs(envs []ExecutionEnvironment) []string {
	ids := make([]string, 0, len(envs))
	for _, ee := range envs {
		ids = append(ids, ee.ID)
	}

	return ids
}
