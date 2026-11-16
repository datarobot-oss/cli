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
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/apiclient"
)

// useCaseLinkPage is how many linked Use Cases one read asks for. The list
// is read one page only (see WorkloadUseCases), so this is also the most a
// caller can see.
const useCaseLinkPage = 100

// UseCaseLinks is what the platform says a workload is linked to.
type UseCaseLinks struct {
	// IDs are the Use Cases on the one page read, in the order returned.
	IDs []string

	// More reports that the platform has more links than the page held. The
	// list's next link drops the entity filter, so following it would return
	// Use Cases unrelated to the workload; a caller treats More as "the
	// answer may be incomplete" instead.
	More bool
}

// Has reports whether id is among the links read.
func (l UseCaseLinks) Has(id string) bool {
	for _, linked := range l.IDs {
		if linked == id {
			return true
		}
	}

	return false
}

// WorkloadUseCases reads the Use Cases a workload is linked to. The workload
// document does not carry its links, which the platform stores beside it, so
// this is the only way to know them.
func WorkloadUseCases(workloadID string) (UseCaseLinks, error) {
	query := url.Values{}
	query.Set("entityId", workloadID)
	query.Set("entityType", "workload")
	query.Set("limit", strconv.Itoa(useCaseLinkPage))

	pageURL, err := drapi.EndpointURL("/useCases/", query)
	if err != nil {
		return UseCaseLinks{}, err
	}

	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Next *string `json:"next"`
	}

	if err := drapi.GetJSON(pageURL, "use cases", &page); err != nil {
		return UseCaseLinks{}, apiclient.LiftDetail(err)
	}

	links := UseCaseLinks{More: page.Next != nil && *page.Next != ""}

	for _, item := range page.Data {
		if item.ID != "" {
			links.IDs = append(links.IDs, item.ID)
		}
	}

	return links, nil
}

// LinkWorkloadToUseCase adds a workload to a Use Case. Linking one that is
// already linked can store a second link record, so a caller checks
// WorkloadUseCases first; a refusal that says the link already exists is
// success, which covers the case where the check could not see every link.
func LinkWorkloadToUseCase(useCaseID, workloadID string) error {
	err := useCaseLinkRequest(http.MethodPost, useCaseID, workloadID)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already linked") {
		return nil
	}

	return err
}

// UseCaseEnclaveCount reads how many Enclaves a Use Case has. The platform
// refuses to create a workload in a Use Case with Enclaves unless the
// workload says how it is placed on them, but it does not check that when an
// existing workload is linked, so a caller linking one asks first.
func UseCaseEnclaveCount(useCaseID string) (int, error) {
	if strings.TrimSpace(useCaseID) == "" {
		return 0, errors.New("reading a Use Case needs its id")
	}

	target, err := drapi.EndpointURL("/useCases/"+escapeID(useCaseID)+"/", nil)
	if err != nil {
		return 0, err
	}

	var uc struct {
		// Missing or null reads as none, as the platform reads it.
		EnclavesCount *int `json:"enclavesCount"`
	}

	if err := drapi.GetJSON(target, "use case", &uc); err != nil {
		return 0, apiclient.LiftDetail(err)
	}

	if uc.EnclavesCount == nil {
		return 0, nil
	}

	return *uc.EnclavesCount, nil
}

// useCaseLinkRequest sends the link route a bodiless request and accepts any
// 2xx: the route answers a link with no content, which drapi.Post does not
// count as success.
func useCaseLinkRequest(method, useCaseID, workloadID string) error {
	if strings.TrimSpace(useCaseID) == "" || strings.TrimSpace(workloadID) == "" {
		return errors.New("a Use Case link needs both a Use Case id and a workload id")
	}

	target, err := drapi.EndpointURL(
		"/useCases/"+escapeID(useCaseID)+"/workloads/"+escapeID(workloadID)+"/", nil)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return err
	}

	if err := drapi.AuthorizeRequest(req); err != nil {
		return err
	}

	resp, err := drapi.Do(req, 0)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiclient.LiftDetail(drapi.ErrFromResp(resp, target))
	}

	// Drained, so the connection is reused for the next link request.
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.Body.Close()
}
