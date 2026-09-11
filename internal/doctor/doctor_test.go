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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCheck is a Check with canned output, used across runner and report tests.
type stubCheck struct {
	id, name string

	res Result
}

func (s stubCheck) ID() string   { return s.id }
func (s stubCheck) Name() string { return s.name }

func (s stubCheck) Run(_ context.Context) Result { return s.res }

func TestRunner_PreservesCheckOrder(t *testing.T) {
	checks := []Check{
		stubCheck{id: "b.check", name: "B", res: Result{Status: StatusOK, Summary: "b"}},
		stubCheck{id: "a.check", name: "A", res: Result{Status: StatusOK, Summary: "a"}},
		stubCheck{id: "c.check", name: "C", res: Result{Status: StatusFAIL, Summary: "c"}},
	}

	results := NewRunner(checks...).Run(context.Background())

	require.Len(t, results, 3)

	got := make([]string, 0, len(results))

	for _, res := range results {
		got = append(got, res.CheckID)
	}

	assert.Equal(t, []string{"b.check", "a.check", "c.check"}, got)
}

func TestRunner_SetsCheckIDFromCheck(t *testing.T) {
	// A check that returns a Result without a CheckID still gets one stamped
	// by the runner, so reporters never render an anonymous row.
	c := stubCheck{id: "x.y", name: "X", res: Result{Status: StatusOK, Summary: "fine"}}

	results := NewRunner(c).Run(context.Background())

	require.Len(t, results, 1)

	assert.Equal(t, "x.y", results[0].CheckID)
}

func TestRunner_EmptyChecks(t *testing.T) {
	results := NewRunner().Run(context.Background())

	assert.Empty(t, results)
}

func TestReport_ExitCode(t *testing.T) {
	cases := []struct {
		name   string
		checks []Result
		want   int
	}{
		{"no checks", nil, 0},
		{"all ok", []Result{{CheckID: "a", Status: StatusOK}}, 0},
		{"warn only", []Result{{CheckID: "a", Status: StatusWARN}}, 0},
		{"skip only", []Result{{CheckID: "a", Status: StatusSKIP}}, 0},
		{"mixed without fail", []Result{
			{CheckID: "a", Status: StatusOK},
			{CheckID: "b", Status: StatusWARN},
			{CheckID: "c", Status: StatusSKIP},
		}, 0},
		{"fail present", []Result{
			{CheckID: "a", Status: StatusOK},
			{CheckID: "b", Status: StatusFAIL},
		}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := NewReport("/tmp/x", nil, tc.checks)

			assert.Equal(t, tc.want, report.ExitCode())
		})
	}
}

func TestReport_OverallStatus(t *testing.T) {
	cases := []struct {
		name   string
		checks []Result
		want   string
	}{
		{"no checks", nil, "ok"},
		{"all ok", []Result{{CheckID: "a", Status: StatusOK}}, "ok"},
		{"skip only counts as ok", []Result{{CheckID: "a", Status: StatusSKIP}}, "ok"},
		{"warn present", []Result{{CheckID: "a", Status: StatusWARN}}, "warn"},
		{"fail beats warn", []Result{
			{CheckID: "a", Status: StatusWARN},
			{CheckID: "b", Status: StatusFAIL},
		}, "fail"},
		{"fail beats ok", []Result{
			{CheckID: "a", Status: StatusOK},
			{CheckID: "b", Status: StatusFAIL},
		}, "fail"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := NewReport("/tmp/x", nil, tc.checks)

			assert.Equal(t, tc.want, report.OverallStatus())
		})
	}
}

func TestReport_Counts(t *testing.T) {
	checks := []Result{
		{CheckID: "a", Status: StatusOK},
		{CheckID: "b", Status: StatusOK},
		{CheckID: "c", Status: StatusWARN},
		{CheckID: "d", Status: StatusFAIL},
		{CheckID: "e", Status: StatusSKIP},
		{CheckID: "f", Status: StatusSKIP},
	}

	report := NewReport("/tmp/x", nil, checks)

	assert.Equal(t, Counts{OK: 2, WARN: 1, FAIL: 1, SKIP: 2}, report.Counts())
}

func TestReport_CountsAlwaysMatchChecks(t *testing.T) {
	// For every combination of statuses, the tally must equal the checks slice.
	statuses := []Status{StatusOK, StatusWARN, StatusFAIL, StatusSKIP}

	for _, a := range statuses {
		for _, b := range statuses {
			checks := []Result{
				{CheckID: "a", Status: a},
				{CheckID: "b", Status: b},
			}

			got := NewReport("/tmp/x", nil, checks).Counts()

			var want Counts

			for _, c := range checks {
				switch c.Status {
				case StatusOK:
					want.OK++
				case StatusWARN:
					want.WARN++
				case StatusFAIL:
					want.FAIL++
				case StatusSKIP:
					want.SKIP++
				}
			}

			assert.Equal(t, want, got, "statuses %s/%s", a, b)
		}
	}
}

func TestAction_Statuses(t *testing.T) {
	// The repair-run action vocabulary is pinned by the output contract.
	assert.Equal(t, ActionPerformed, ActionStatus("performed"))
	assert.Equal(t, ActionSkipped, ActionStatus("skipped"))
	assert.Equal(t, ActionNotNeeded, ActionStatus("not-needed"))
}
