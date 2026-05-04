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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

type Client interface {
	CreateCatalog() (*CatalogResp, error)
	CreateStage(catalogID string) (*StageResp, error)
	UploadToStage(catalogID, stageID, name string, size int64, body io.Reader) error
	ApplyStage(catalogID, stageID, overwrite string) (*ApplyStageResp, error)
	UploadFromZipNew(name string, size int64, body io.Reader) (*FromFileResp, error)
	UploadFromZipExisting(catalogID, name, overwrite string, size int64, body io.Reader) (*FromFileResp, error)
	PollStatus(statusID string) (*StatusResp, error)
	AllFiles(catalogID, versionID string) (map[string]FileMeta, error)
	DownloadFile(catalogID, versionID, path string, w io.Writer) (string, int64, error)
	DeleteFiles(catalogID string, paths []string) (*DeleteFilesResp, error)
}

func NewClient() Client {
	return &httpClient{}
}

type httpClient struct{}

func endpointURL(path string, query url.Values) (string, error) {
	full, err := config.GetEndpointURL("/api/v2" + path)
	if err != nil {
		return "", err
	}

	if len(query) == 0 {
		return full, nil
	}

	return full + "?" + query.Encode(), nil
}

func errFromResp(resp *http.Response, requestURL string) error {
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if len(body) > 0 {
		return fmt.Errorf("%w: body=%s", &drapi.HTTPError{StatusCode: resp.StatusCode, URL: requestURL}, body)
	}

	return &drapi.HTTPError{StatusCode: resp.StatusCode, URL: requestURL}
}

// decorateAuthHeaders adds the same auth/telemetry headers drapi applies,
// for the multipart paths that build their own requests.
func decorateAuthHeaders(req *http.Request) error {
	token, err := config.GetAPIKey(context.Background())
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	if config.IsAPIConsumerTrackingEnabled() {
		req.Header.Set("X-DataRobot-Api-Consumer-Trace", config.GetAPIConsumerTrace())
	}

	return nil
}
