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

package filesapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/datarobot/cli/internal/drapi"
)

func (c *httpClient) UploadFromZipNew(name string, size int64, body io.Reader) (*FromFileResp, error) {
	q := url.Values{}
	q.Set("useArchiveContents", "true")

	requestURL, err := drapi.EndpointURL("/files/fromFile/", q)
	if err != nil {
		return nil, fmt.Errorf("build files url: %w", err)
	}

	return uploadZipMultipart(requestURL, name, size, body)
}

func (c *httpClient) UploadFromZipExisting(catalogID, name, overwrite string, size int64, body io.Reader) (*FromFileResp, error) {
	if overwrite == "" {
		overwrite = OverwriteReplace
	}

	q := url.Values{}
	q.Set("useArchiveContents", "true")
	q.Set("overwrite", overwrite)

	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/fromFile/", q)
	if err != nil {
		return nil, fmt.Errorf("build fromFile url: %w", err)
	}

	return uploadZipMultipart(requestURL, name, size, body)
}

func uploadZipMultipart(requestURL, name string, size int64, body io.Reader) (*FromFileResp, error) {
	req, err := newStreamingMultipartRequest(requestURL, nil, name, size, body)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: uploadHTTPTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zip upload %s: %w", name, err)
	}

	defer func() { _ = resp.Body.Close() }()

	// 202 async, 201 sync (small archives), 200 occasionally.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, drapi.ErrFromResp(resp, requestURL)
	}

	var out FromFileResp

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode fromFile response: %w", err)
	}

	return &out, nil
}

func (c *httpClient) PollStatus(statusID string) (*StatusResp, error) {
	requestURL, err := drapi.EndpointURL("/status/"+statusID+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("build status url: %w", err)
	}

	httpResp, err := getAcceptingRedirect(requestURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == http.StatusSeeOther {
		// 303 means COMPLETED with empty body — synthesize a StatusResp.
		return &StatusResp{Status: StatusCompleted, StatusID: statusID}, nil
	}

	var resp StatusResp

	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}

	return &resp, nil
}

func getAcceptingRedirect(requestURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build status request: %w", err)
	}

	if err := drapi.AuthorizeRequest(req); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: statusPollHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSeeOther {
		return nil, drapi.ErrFromResp(resp, requestURL)
	}

	return resp, nil
}
