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
	"fmt"
	"io"
	"net/http"

	"github.com/datarobot/cli/internal/drapi"
)

func (c *httpClient) CreateCatalog() (*CatalogResp, error) {
	requestURL, err := drapi.EndpointURL("/files/", nil)
	if err != nil {
		return nil, fmt.Errorf("build catalog url: %w", err)
	}

	var resp CatalogResp

	if err := drapi.PostJSON(requestURL, "catalog", struct{}{}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *httpClient) CreateStage(catalogID string) (*StageResp, error) {
	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/stages/", nil)
	if err != nil {
		return nil, fmt.Errorf("build stage url: %w", err)
	}

	var resp StageResp

	if err := drapi.PostJSON(requestURL, "stage", struct{}{}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *httpClient) UploadToStage(catalogID, stageID, name string, size int64, body io.Reader) error {
	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/stages/"+stageID+"/upload/", nil)
	if err != nil {
		return fmt.Errorf("build upload url: %w", err)
	}

	req, err := newStreamingMultipartRequest(requestURL, nil, name, size, body)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: uploadHTTPTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", name, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return drapi.ErrFromResp(resp, requestURL)
	}

	return nil
}

func (c *httpClient) ApplyStage(catalogID, stageID, overwrite string) (*ApplyStageResp, error) {
	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/fromStage/", nil)
	if err != nil {
		return nil, fmt.Errorf("build apply-stage url: %w", err)
	}

	var resp ApplyStageResp

	body := ApplyStageReq{StageID: stageID, Overwrite: overwrite}
	if err := drapi.PostJSON(requestURL, "apply-stage", body, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
