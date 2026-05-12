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
	"net/url"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/fileops"
)

func (c *httpClient) AllFiles(catalogID, versionID string) (map[string]FileMeta, error) {
	out := make(map[string]FileMeta)

	pageURL, err := allFilesURL(catalogID, versionID)
	if err != nil {
		return nil, err
	}

	for pageURL != "" {
		var page AllFilesResp

		if err := drapi.GetJSON(pageURL, "files", &page); err != nil {
			return nil, err
		}

		for _, item := range page.Data {
			key := fileops.NormalizePath(item.FileName)
			if err := fileops.SafeRelPath(key); err != nil {
				return nil, fmt.Errorf("remote manifest entry %q: %w", item.FileName, err)
			}

			out[key] = FileMeta{Hash: item.FileChecksum, Size: item.FileSize}
		}

		if page.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(page.Next); err != nil {
			return nil, err
		}

		pageURL = page.Next
	}

	return out, nil
}

func allFilesURL(catalogID, versionID string) (string, error) {
	if versionID != "" {
		return drapi.EndpointURL("/files/"+catalogID+"/versions/"+versionID+"/allFiles/", nil)
	}

	return drapi.EndpointURL("/files/"+catalogID+"/allFiles/", nil)
}

func (c *httpClient) DownloadFile(catalogID, versionID, path string, w io.Writer) (string, int64, error) {
	if err := fileops.SafeRelPath(path); err != nil {
		return "", 0, fmt.Errorf("download path %q: %w", path, err)
	}

	q := url.Values{}
	q.Set("fileName", path)

	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/versions/"+versionID+"/file/", q)
	if err != nil {
		return "", 0, fmt.Errorf("build download url: %w", err)
	}

	resp, err := drapi.Get(requestURL, "file", 300)
	if err != nil {
		return "", 0, fmt.Errorf("download %s: %w", path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return "", n, fmt.Errorf("write %s: %w", path, err)
	}

	return "", n, nil
}

func (c *httpClient) DeleteFiles(catalogID string, paths []string) (*DeleteFilesResp, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	requestURL, err := drapi.EndpointURL("/files/"+catalogID+"/allFiles/", nil)
	if err != nil {
		return nil, fmt.Errorf("build delete url: %w", err)
	}

	var resp DeleteFilesResp

	if err := drapi.DeleteJSON(requestURL, "files", DeleteFilesReq{Paths: paths}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
