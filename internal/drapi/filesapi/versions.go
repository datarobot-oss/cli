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
	"net/url"
	"strconv"

	"github.com/datarobot/cli/internal/drapi"
)

// versionsPageSize controls the per-request page size for ListVersions.
// The server caps this server-side; we set the requested page size to a
// large round number so a typical artifact's full history fits in one or
// two round trips.
const versionsPageSize = 100

// ListVersions returns the catalog's version history newest-first. When
// limit > 0 the result is truncated to that many entries (pagination
// stops as soon as the cap is reached). When limit <= 0 every page is
// followed until the server returns no Next cursor.
func (c *httpClient) ListVersions(catalogID string, limit int) ([]CatalogVersion, error) {
	q := url.Values{}
	q.Set("orderBy", "-created")
	q.Set("limit", strconv.Itoa(versionsPageSize))

	pageURL, err := drapi.EndpointURL("/files/"+catalogID+"/versions/", q)
	if err != nil {
		return nil, err
	}

	out := make([]CatalogVersion, 0)

	for pageURL != "" {
		var page CatalogVersionsResp

		if err := drapi.GetJSON(pageURL, "versions", &page); err != nil {
			return nil, err
		}

		for _, v := range page.Data {
			out = append(out, v)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
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
