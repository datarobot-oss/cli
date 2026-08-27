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

// Package paginate provides the shared next-link pagination walk for list
// endpoints that answer one page per request plus a link to the next one.
// Callers keep their own envelope types, per-endpoint page-size clamps, and
// next-link host checks; Walk owns only the loop: accumulate rows, stop once
// limit rows are collected or the pages run out, and return at most limit
// rows.
package paginate

import (
	"fmt"
)

// Walk fetches successive pages starting at startURL and returns up to limit
// rows. fetch decodes one page into the caller's envelope shape and returns
// its rows plus the next-page URL, or "" when there are no more pages; it
// also owns per-request details such as the clamped page size. limit must be
// positive: a caller asking for zero rows is a bug, not an empty result.
func Walk[T any](startURL string, limit int, fetch func(pageURL string) (rows []T, next string, err error)) ([]T, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	var all []T

	pageURL := startURL

	for pageURL != "" {
		rows, next, err := fetch(pageURL)
		if err != nil {
			return nil, err
		}

		all = append(all, rows...)

		if len(all) >= limit {
			return all[:limit], nil
		}

		pageURL = next
	}

	return all, nil
}
