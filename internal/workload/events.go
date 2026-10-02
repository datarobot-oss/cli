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
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/drapi"
)

// WorkloadEvent is one entry of a workload's lifecycle trail: what changed,
// when, and who did it. Details is the platform's own account of the event
// and differs by type, so it is kept as it came and re-emitted under JSON.
type WorkloadEvent struct {
	ID         string          `json:"id"`
	WorkloadID string          `json:"workloadId"`
	Timestamp  time.Time       `json:"timestamp"`
	EventType  string          `json:"eventType"`
	ActorID    string          `json:"actorId"`
	Details    json.RawMessage `json:"details"`
}

// Message is the sentence the platform wrote for the event, "" when the
// details carry none.
func (e WorkloadEvent) Message() string {
	var details struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(e.Details, &details); err != nil {
		return ""
	}

	return details.Message
}

// ArtifactID is the artifact a replacement event rolled onto, "" for an
// event that names none.
func (e WorkloadEvent) ArtifactID() string {
	var details struct {
		ArtifactID string `json:"artifactId"`
	}

	if err := json.Unmarshal(e.Details, &details); err != nil {
		return ""
	}

	return details.ArtifactID
}

// ReplacementID is the replacement a replacement event records, read from
// the details and falling back to the event's own id, which the platform
// sets to the same value.
func (e WorkloadEvent) ReplacementID() string {
	var details struct {
		ReplacementID string `json:"replacementId"`
	}

	if err := json.Unmarshal(e.Details, &details); err == nil && details.ReplacementID != "" {
		return details.ReplacementID
	}

	return e.ID
}

// ReplacementStatus is how the replacement ended, in the platform's lower
// case word: "completed", "errored", "failed", "cancelled". The event type
// is "Replacement <Status>", so it is the type with the noun taken off.
func (e WorkloadEvent) ReplacementStatus() string {
	status := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(e.EventType), "replacement"))

	return status
}

// CandidateProtonIDs are the generations a replacement event says were
// launched, nil for an event that names none.
func (e WorkloadEvent) CandidateProtonIDs() []string {
	var details struct {
		CandidateProtonIDs []string `json:"candidateProtonIds"`
	}

	if err := json.Unmarshal(e.Details, &details); err != nil {
		return nil
	}

	return details.CandidateProtonIDs
}

// RolloutRecord is the platform's record of started once it has finished,
// nil when the trail has none for it yet. The trail is where finished
// replacements end up, written before the workload's active slot is cleared,
// so a record the active route no longer answers for is here.
//
// The finished record is written under a new id, so it is found by what the
// replacement launched: a record naming one of its generations is it. A
// replacement that never launched any is matched by its artifact among the
// records written after it began; the record's id carries its creation time,
// where the event's timestamp carries the record's last update, which a
// cleanup pass bumps minutes later.
func RolloutRecord(workloadID string, started *Replacement) (*WorkloadEvent, error) {
	if started == nil {
		return nil, nil
	}

	events, err := ListWorkloadEvents(workloadID, maxPageSize, EventFilter{Types: []string{"replacement"}})
	if err != nil {
		return nil, err
	}

	var (
		found *WorkloadEvent
		at    time.Time
	)

	for i := range events {
		e := &events[i]
		if !recordsRollout(e, started) {
			continue
		}

		if when := recordedAt(*e); found == nil || when.After(at) {
			found, at = e, when
		}
	}

	return found, nil
}

// recordsRollout reports whether e is started's finished record: it names
// one of the generations started launched, or, when started launched none,
// it is a record of the same artifact written after started began.
func recordsRollout(e *WorkloadEvent, started *Replacement) bool {
	if len(started.CandidateProtonIDs) > 0 {
		launched := e.CandidateProtonIDs()

		return slices.ContainsFunc(started.CandidateProtonIDs, func(id string) bool {
			return slices.Contains(launched, id)
		})
	}

	if started.ArtifactID == "" || e.ArtifactID() != started.ArtifactID {
		return false
	}

	// A minute of slack: the two clocks are the same server's, but a record
	// can be written in the second the replacement was.
	return started.CreatedAt.IsZero() || !recordedAt(*e).Before(started.CreatedAt.Add(-time.Minute))
}

// recordedAt is when a replacement event's record was written, from its
// ObjectId, with the event's timestamp standing in for an id of another shape.
func recordedAt(e WorkloadEvent) time.Time {
	id := e.ReplacementID()
	if len(id) != 24 {
		return e.Timestamp
	}

	seconds, err := strconv.ParseInt(id[:8], 16, 64)
	if err != nil {
		return e.Timestamp
	}

	return time.Unix(seconds, 0)
}

type workloadEventList struct {
	Data []WorkloadEvent `json:"data"`
	Next string          `json:"next"`
}

// maxEventPages bounds the walk through a workload's trail. A trail is a
// few dozen entries on a busy workload; anything approaching this is a route
// that is not ending rather than a workload with an unusual history.
const maxEventPages = 50

// EventFilter narrows a trail. The zero value keeps everything.
//
// Everything here is applied client-side. The events route takes limit and
// offset and nothing else: every filter parameter proposed for it was
// refused with extra_forbidden when measured on staging.
type EventFilter struct {
	// Types are substrings an event's type must contain, any of them,
	// matched without regard to case, so "replacement" finds both
	// "Replacement Completed" and "Replacement Errored".
	Types []string

	// ProtonID keeps the events whose details mention the generation, by id.
	ProtonID string

	// Since and Until bound the window. Zero means unbounded on that side.
	Since time.Time
	Until time.Time
}

func (f EventFilter) admits(e WorkloadEvent) bool {
	if len(f.Types) > 0 && !slices.ContainsFunc(f.Types, func(t string) bool {
		return strings.Contains(strings.ToLower(e.EventType), strings.ToLower(t))
	}) {
		return false
	}

	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}

	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}

	return f.ProtonID == "" || mentions(e.Details, f.ProtonID)
}

// mentions reports whether id appears as a string anywhere in a details
// document, at any depth. The details differ by event type and name their
// generations under more than one key (candidateProtonIds, protonStatuses),
// so the search is by value rather than by field.
func mentions(details json.RawMessage, id string) bool {
	var doc any

	if err := json.Unmarshal(details, &doc); err != nil {
		return false
	}

	return valueMentions(doc, id)
}

func valueMentions(v any, id string) bool {
	switch v := v.(type) {
	case string:
		return v == id
	case []any:
		return slices.ContainsFunc(v, func(item any) bool { return valueMentions(item, id) })
	case map[string]any:
		for key, item := range v {
			if key == id || valueMentions(item, id) {
				return true
			}
		}
	}

	return false
}

// ListWorkloadEvents returns the most recent limit events that pass filter,
// oldest first.
//
// The whole trail is read, filtered here and sorted by time, because the
// route filters by nothing and does not promise an order: the entries of one
// rollout came back a few milliseconds out of sequence on staging. A trail
// is short, so reading it whole costs a page or two; the bound on pages is
// there for a route that stops ending, not for a workload with history.
func ListWorkloadEvents(workloadID string, limit int, filter EventFilter) ([]WorkloadEvent, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(maxPageSize))

	pageURL, err := drapi.EndpointURL("/workloads/"+escapeID(workloadID)+"/events/", query)
	if err != nil {
		return nil, err
	}

	all, err := drainEventPages(workloadID, pageURL)
	if err != nil {
		return nil, err
	}

	kept := make([]WorkloadEvent, 0, len(all))

	for _, e := range all {
		if filter.admits(e) {
			kept = append(kept, e)
		}
	}

	slices.SortStableFunc(kept, func(a, b WorkloadEvent) int { return a.Timestamp.Compare(b.Timestamp) })

	if len(kept) > limit {
		kept = kept[len(kept)-limit:]
	}

	return kept, nil
}

// drainEventPages follows next-links from pageURL until the trail ends.
func drainEventPages(workloadID, pageURL string) ([]WorkloadEvent, error) {
	var all []WorkloadEvent

	for page := 0; pageURL != ""; page++ {
		if page >= maxEventPages {
			return nil, fmt.Errorf("the event trail of workload %s did not end after %d pages; refusing to follow more",
				workloadID, maxEventPages)
		}

		var list workloadEventList

		if err := drapi.GetJSON(pageURL, "workload events", &list); err != nil {
			return nil, err
		}

		all = append(all, list.Data...)

		if list.Next == "" || len(list.Data) == 0 {
			break
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return nil, err
		}

		pageURL = list.Next
	}

	return all, nil
}
