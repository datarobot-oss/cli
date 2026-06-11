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
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
)

// WorkloadLogEntry is one log line from the workload's running container(s).
// Timestamp is kept as the server's raw string (e.g. "2026-06-11
// 14:04:14.223413+00:00", space-separated with microseconds and a +00:00
// offset, not RFC3339) because the CLI only displays it.
type WorkloadLogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

type workloadLogsResponse struct {
	Data     []WorkloadLogEntry `json:"data"`
	Count    int                `json:"count"`
	Next     string             `json:"next"`
	Previous string             `json:"previous"`
}

// GetWorkloadLogs returns up to limit of the most recent log lines for a
// workload, reversed to chronological order (oldest first) for display, like
// `kubectl logs --tail`. level filters by minimum severity (debug, the
// server default, returns everything); an empty level leaves the server
// default in place.
//
// The logs endpoint lives under the public gateway at /api/v2/otel/..., not
// in workload-api's route table, and the gateway camelizes query keys, so
// searchKeys/searchValues are used (snake_case is rejected with 400). proton
// and time-range filters are optional and omitted here: the bare call
// returns the workload's current logs.
func GetWorkloadLogs(workloadID string, limit int, level string) ([]WorkloadLogEntry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxWorkloadPageSize)))

	if level != "" {
		query.Set("level", strings.ToLower(level))
	}

	pageURL, err := drapi.EndpointURL("/otel/workload/"+escapeID(workloadID)+"/logs/", query)
	if err != nil {
		return nil, err
	}

	var all []WorkloadLogEntry

	for pageURL != "" {
		var resp workloadLogsResponse

		if err := drapi.GetJSON(pageURL, "workload logs", &resp); err != nil {
			return nil, err
		}

		all = append(all, resp.Data...)

		if len(all) >= limit {
			all = all[:limit]

			break
		}

		if resp.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(resp.Next); err != nil {
			return nil, err
		}

		pageURL = resp.Next
	}

	// The server returns newest first; reverse so the most recent line is
	// last, the natural reading order for logs.
	reverseLogs(all)

	return all, nil
}

func reverseLogs(entries []WorkloadLogEntry) {
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
}

// logKey identifies a log line for the follow-mode dedup. Timestamp alone is
// not unique (a process can emit several lines in the same microsecond), so
// the message is included.
func logKey(e WorkloadLogEntry) string {
	return e.Timestamp + "\x00" + e.Message
}

// FilterUnseenLogs returns the entries whose key is not already in seen,
// recording them in seen as a side effect. `dr workload logs --wait` calls it
// each poll so only lines not shown on a prior poll are printed. Input order
// is preserved (the caller passes chronological entries).
func FilterUnseenLogs(entries []WorkloadLogEntry, seen map[string]struct{}) []WorkloadLogEntry {
	fresh := make([]WorkloadLogEntry, 0, len(entries))

	for _, e := range entries {
		key := logKey(e)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		fresh = append(fresh, e)
	}

	return fresh
}
