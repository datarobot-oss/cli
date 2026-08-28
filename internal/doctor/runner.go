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

package doctor

import "context"

// Runner executes an ordered list of checks. Order is the caller's
// responsibility and is preserved end-to-end: results and both reporters
// render checks in construction order.
type Runner struct {
	checks []Check
}

// NewRunner builds a Runner that executes the given checks in the given order.
func NewRunner(checks ...Check) *Runner {
	return &Runner{checks: checks}
}

// Run executes every check in construction order and returns the results in
// the same order. Each result's CheckID is stamped from the check itself, so
// reporters never render an anonymous row even if a check forgets to set it.
func (r *Runner) Run(ctx context.Context) []Result {
	results := make([]Result, 0, len(r.checks))

	for _, check := range r.checks {
		res := check.Run(ctx)

		res.CheckID = check.ID()

		results = append(results, res)
	}

	return results
}

// Counts is the per-status tally of a run's checks, serialized with the
// pinned lowercase keys in the JSON summary.
type Counts struct {
	OK   int `json:"ok"`
	WARN int `json:"warn"`
	FAIL int `json:"fail"`
	SKIP int `json:"skip"`
}

// OverallStatus derives the run's top-level verdict from its checks:
// "fail" if any check FAILed, else "warn" if any WARNed, else "ok".
// A run with only SKIPs counts as ok.
func OverallStatus(checks []Result) string {
	counts := CountResults(checks)

	switch {
	case counts.FAIL > 0:
		return "fail"
	case counts.WARN > 0:
		return "warn"
	default:
		return "ok"
	}
}

// CountResults tallies checks by status.
func CountResults(checks []Result) Counts {
	var counts Counts

	for _, res := range checks {
		switch res.Status {
		case StatusOK:
			counts.OK++
		case StatusWARN:
			counts.WARN++
		case StatusFAIL:
			counts.FAIL++
		case StatusSKIP:
			counts.SKIP++
		}
	}

	return counts
}
