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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/apiclient"
)

// useCaseLinkPage is how many linked Use Cases one request asks for.
const useCaseLinkPage = 100

// useCaseLinkPages bounds the pages one read follows, so a server that keeps
// answering full pages cannot hold a deploy forever. It is far past any
// workload's real number of links.
const useCaseLinkPages = 100

// UseCaseLinks is what the platform says a workload is linked to.
type UseCaseLinks struct {
	// IDs are the Use Cases the workload is linked to, in the order returned.
	IDs []string
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

// WorkloadUseCases reads every Use Case a workload is linked to. The workload
// document does not carry its links, which the platform stores beside it, so
// this is the only way to know them.
//
// The list pages by offset. Its next link drops the entity filter, so instead
// of following it each page is asked for again with the filter and the next
// offset, until a page comes back short or totalCount is reached.
func WorkloadUseCases(workloadID string) (UseCaseLinks, error) {
	var links UseCaseLinks

	for page := range useCaseLinkPages {
		data, total, err := useCaseLinkPageAt(workloadID, page*useCaseLinkPage)
		if err != nil {
			return UseCaseLinks{}, err
		}

		for _, id := range data {
			if id != "" {
				links.IDs = append(links.IDs, id)
			}
		}

		read := page*useCaseLinkPage + len(data)

		if len(data) < useCaseLinkPage || (total != nil && read >= *total) {
			return links, nil
		}
	}

	return UseCaseLinks{}, fmt.Errorf(
		"workload %s has more than %d Use Case links; stopped reading them", workloadID, useCaseLinkPage*useCaseLinkPages)
}

// useCaseLinkPageAt reads one page of a workload's links from offset, and the
// total the platform reports, nil when it reports none.
func useCaseLinkPageAt(workloadID string, offset int) ([]string, *int, error) {
	query := url.Values{}
	query.Set("entityId", workloadID)
	query.Set("entityType", "workload")
	query.Set("limit", strconv.Itoa(useCaseLinkPage))
	query.Set("offset", strconv.Itoa(offset))

	pageURL, err := drapi.EndpointURL("/useCases/", query)
	if err != nil {
		return nil, nil, err
	}

	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		TotalCount *int `json:"totalCount"`
	}

	if err := drapi.GetJSON(pageURL, "use cases", &page); err != nil {
		return nil, nil, apiclient.LiftDetail(err)
	}

	ids := make([]string, 0, len(page.Data))

	for _, item := range page.Data {
		ids = append(ids, item.ID)
	}

	return ids, page.TotalCount, nil
}

// ErrUseCaseUnavailable marks a Use Case the platform answered 403 or 404 for.
// The monolith's Use Case routes put their reason in a body the client does
// not lift, so without this the user would read a bare HTTP status, which
// looks like a transient failure rather than a wrong id or missing access.
var ErrUseCaseUnavailable = errors.New("doesn't exist or you don't have access to it")

// useCaseUnavailable names a 403 or 404 from a Use Case route for what it is,
// and passes any other error through as it came.
func useCaseUnavailable(useCaseID string, err error) error {
	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	if httpErr.StatusCode != http.StatusNotFound && httpErr.StatusCode != http.StatusForbidden {
		return err
	}

	return fmt.Errorf("the Use Case %s %w (HTTP %d)", useCaseID, ErrUseCaseUnavailable, httpErr.StatusCode)
}

// LinkWorkloadToUseCase adds a workload to a Use Case. Linking one that is
// already linked can store a second link record, so a caller checks
// WorkloadUseCases first, which reads every link.
func LinkWorkloadToUseCase(useCaseID, workloadID string) error {
	return useCaseLinkRequest(http.MethodPost, useCaseID, workloadID)
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
		return 0, useCaseUnavailable(useCaseID, apiclient.LiftDetail(err))
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
		return useCaseUnavailable(useCaseID, apiclient.LiftDetail(drapi.ErrFromResp(resp, target)))
	}

	// Drained, so the connection is reused for the next link request.
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.Body.Close()
}
