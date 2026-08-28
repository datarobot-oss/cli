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

// Package doctor is a generic, state-agnostic check-and-report framework.
//
// It defines a Check interface, a Result type, an ordered Runner, and two
// reporters (human-readable text and pure-JSON). It knows nothing about any
// specific state model: concrete checks (e.g. workload sync-state checks)
// live in their own packages and plug into the Runner. This keeps the layer
// reusable for a future top-level "dr doctor".
package doctor

import "context"

// Status is the outcome of a single check. Check-level statuses are rendered
// uppercase in both reporters; the run's overall verdict (derived from these)
// is rendered lowercase.
type Status string

const (
	// StatusOK means the condition checked for is healthy.
	StatusOK Status = "OK"

	// StatusWARN means something needs attention but the run can proceed.
	StatusWARN Status = "WARN"

	// StatusFAIL means the condition checked for is broken; any FAIL makes
	// the overall exit code 1.
	StatusFAIL Status = "FAIL"

	// StatusSKIP means the check could not meaningfully run (e.g. an earlier
	// check failed and this one depends on it). SKIP is honest reporting,
	// never a silent pass.
	StatusSKIP Status = "SKIP"
)

// Result is the outcome of one check. CheckID is normally stamped by the
// Runner from the Check's ID, so individual checks do not need to set it.
type Result struct {
	// CheckID is the stable namespaced identifier of the check (e.g.
	// "wapi.config"). It matches Check.ID.
	CheckID string

	// Status is the check outcome.
	Status Status

	// Summary is a one-line human-readable description of the finding.
	Summary string

	// Remedy is the canonical remedy string for a non-OK result; empty for OK.
	Remedy string

	// Details holds optional structured extras (e.g. {"path": "/abs/file"})
	// surfaced in JSON output; omitted when nil.
	Details map[string]string

	// Fixable reports whether "doctor --fix" can repair this condition.
	Fixable bool
}

// Check is a single diagnostic. Implementations are pure diagnostics: they
// MUST NOT mutate local state or perform server writes; repairs live behind
// explicit repair operations in the owning command layer.
type Check interface {
	// ID returns the stable namespaced identifier (e.g. "wapi.presence").
	ID() string

	// Name returns a human-readable name for display.
	Name() string

	// Run executes the check and returns its Result.
	Run(ctx context.Context) Result
}

// ActionStatus is the outcome of one repair operation in a repair run
// (--fix / --relink).
type ActionStatus string

const (
	// ActionPerformed means the repair was executed successfully.
	ActionPerformed ActionStatus = "performed"

	// ActionSkipped means the repair was not executed, with Reason saying why.
	ActionSkipped ActionStatus = "skipped"

	// ActionNotNeeded means the repair had nothing to do (already healthy).
	ActionNotNeeded ActionStatus = "not-needed"
)

// Action describes one repair operation for the reporters' actions section.
type Action struct {
	// ID is the check or operation identifier this action belongs to.
	ID string `json:"id"`

	// Status is performed, skipped, or not-needed.
	Status ActionStatus `json:"status"`

	// Reason explains a skip (or other non-obvious outcome); omitted when empty.
	Reason string `json:"reason,omitempty"`
}
