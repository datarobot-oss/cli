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
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/paginate"
)

// jsonPage is the wire shape shared by the workload-family list endpoints:
// rows under "data" plus a next-page link. Count, TotalCount, and Previous
// were decoded by the former per-endpoint envelopes but never read, so the
// generic shape omits them.
type jsonPage[T any] struct {
	Data []T    `json:"data"`
	Next string `json:"next"`
}

// listWalk accumulates rows from a paginated workload-family endpoint: it
// GETs each page with label for the request log, host-checks every next link
// before following it, and stops once limit rows are collected or the pages
// run out. It is the single implementation behind ListWorkloads,
// ListArtifacts, and ListArtifactBuilds, which previously each carried their
// own copy of the loop.
func listWalk[T any](startURL, label string, limit int) ([]T, error) {
	return paginate.Walk(startURL, limit, func(pageURL string) ([]T, string, error) {
		var page jsonPage[T]

		if err := drapi.GetJSON(pageURL, label, &page); err != nil {
			return nil, "", err
		}

		if page.Next != "" {
			if err := drapi.AssertNextOnSameHost(page.Next); err != nil {
				return nil, "", err
			}
		}

		return page.Data, page.Next, nil
	})
}
