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
	"net/url"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/usecase"
)

// UseCase is the part of a DataRobot Use Case the workload commands read.
type UseCase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LatestUseCase is the most recently updated Use Case the caller can see, nil
// when there is none.
func LatestUseCase() (*UseCase, error) {
	return firstUseCase(url.Values{"orderBy": {"-updatedAt"}})
}

// WorkloadUseCase is the Use Case a workload is linked to, nil when it is
// linked to none. The Workload API keeps the link in the Use Case service and
// does not echo it on the workload, so this asks the Use Case side; a
// workload carries one link, which is why the first row is the answer.
func WorkloadUseCase(workloadID string) (*UseCase, error) {
	return firstUseCase(url.Values{"entityId": {workloadID}, "entityType": {"workload"}})
}

func firstUseCase(query url.Values) (*UseCase, error) {
	query.Set("limit", "1")

	pageURL, err := drapi.EndpointURL("/useCases/", query)
	if err != nil {
		return nil, err
	}

	var page struct {
		Data []UseCase `json:"data"`
	}

	if err := drapi.GetJSON(pageURL, "use cases", &page); err != nil {
		return nil, err
	}

	if len(page.Data) == 0 {
		return nil, nil
	}

	return &page.Data[0], nil
}

// UseCaseHasEnclaves reports whether an administrator has granted the Use
// Case any Enclave. The Workload API refuses to create a workload linked to
// such a Use Case unless the spec sets an enclaveSelectionPolicy. A Use Case
// that does not exist is a 404 wrapped in *drapi.HTTPError.
func UseCaseHasEnclaves(id usecase.ID) (bool, error) {
	pageURL, err := drapi.EndpointURL("/useCases/"+escapeID(string(id))+"/resources/",
		url.Values{"entityType": {"enclave"}})
	if err != nil {
		return false, err
	}

	var page struct {
		Data []struct{} `json:"data"`
	}

	if err := drapi.GetJSON(pageURL, "use case enclaves", &page); err != nil {
		return false, err
	}

	return len(page.Data) > 0, nil
}

// LinkWorkloadToUseCase links an existing workload into a Use Case. The
// Workload API takes useCaseId only on create, and a workload placed on an
// Enclave that has lost (or never had) its link cannot be restarted or
// replaced, so this is how a deploy repairs one.
func LinkWorkloadToUseCase(id usecase.ID, workloadID string) error {
	linkURL, err := drapi.EndpointURL(
		"/useCases/"+escapeID(string(id))+"/workloads/"+escapeID(workloadID)+"/", nil)
	if err != nil {
		return err
	}

	resp, err := drapi.Post(linkURL, "use case link", map[string]any{})
	if err != nil {
		return err
	}

	return resp.Body.Close()
}
