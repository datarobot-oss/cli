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

package pipeline

import (
	"fmt"
	"net/http"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/paginate"
)

// pipelinePageSize is the per-request page-size ceiling for the pipelines-api
// list endpoints (DataPage convention). A larger --limit is satisfied by
// walking next-links rather than one oversized request the server would
// clamp or reject.
const pipelinePageSize = 200

// validatePagination rejects non-positive limits and negative offsets for the
// list endpoints, mirroring the workload family's argument checks, so the
// next-link walk never sees out-of-range arguments.
func validatePagination(offset, limit int) error {
	if limit <= 0 {
		return fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	if offset < 0 {
		return fmt.Errorf("invalid offset %d: must be non-negative", offset)
	}

	return nil
}

// walkDataPage walks a DataPage list endpoint starting at firstURL, following
// the envelope's "next" link (host-checked) until limit rows are collected or
// the pages run out, and returns at most limit rows. TotalCount is captured
// from the most recently fetched page — the server states the same total on
// every page — so renderers keep showing the true total next to the returned
// row count.
func walkDataPage[T any](firstURL, label string, limit int) (*DataPage[T], error) {
	var totalCount int

	rows, err := paginate.Walk(firstURL, limit, func(pageURL string) ([]T, string, error) {
		var page DataPage[T]

		if err := doJSON(http.MethodGet, pageURL, nil, label, &page); err != nil {
			return nil, "", err
		}

		totalCount = page.TotalCount

		next := ""

		if page.Next != nil {
			if err := drapi.AssertNextOnSameHost(*page.Next); err != nil {
				return nil, "", err
			}

			next = *page.Next
		}

		return page.Data, next, nil
	})
	if err != nil {
		return nil, err
	}

	return &DataPage[T]{Data: rows, Count: len(rows), TotalCount: totalCount}, nil
}

// walkDataRows is walkDataPage for endpoints whose callers only need the
// rows; it discards the envelope metadata.
func walkDataRows[T any](firstURL, label string, limit int) ([]T, error) {
	page, err := walkDataPage[T](firstURL, label, limit)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}
