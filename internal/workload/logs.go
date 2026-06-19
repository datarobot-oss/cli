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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/drapi"
)

// The logs endpoint is the public gateway's OTEL route
// /api/v2/otel/workload/{id}/logs/: offset-paginated newest-first, with
// camelCase query keys (limit, level, startTime) and next/previous links.

// maxLogsPageSize is the server's per-page limit ceiling (wider than the
// workload list endpoint's 100, so it is its own constant).
const maxLogsPageSize = 1000

// followLagAllowance is how far behind the newest-seen timestamp the follow
// cursor trails, so late-ingested lines are caught by the dedup overlap
// rather than skipped.
const followLagAllowance = 10 * time.Second

// followSeenCap bounds each follow dedup generation so memory stays bounded.
const followSeenCap = 5000

// maxTransientPollErrors caps consecutive transient fetch failures a follow
// tolerates before giving up; it resets on any successful poll.
const maxTransientPollErrors = 5

// sleepInterval waits for interval or ctx cancellation, returning false when
// ctx ended first so Ctrl-C interrupts the follow promptly.
func sleepInterval(ctx context.Context, interval time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(interval):
		return true
	}
}

// isTransientPollError reports whether a fetch failure is worth retrying:
// 5xx, 429, and non-HTTP (network) errors. A 4xx is terminal.
func isTransientPollError(err error) bool {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}

	return true
}

// logLevels are the values the level filter accepts; warn aliases warning.
var logLevels = []string{"debug", "info", "warn", "warning", "error", "critical"}

// ParseLogLevel lowercases and validates a --level value so a typo fails
// locally with the valid set listed. Empty stays empty (server defaults to
// debug).
func ParseLogLevel(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	lower := strings.ToLower(strings.TrimSpace(value))
	if slices.Contains(logLevels, lower) {
		return lower, nil
	}

	return "", fmt.Errorf("invalid log level %q: use one of %s", value, strings.Join(logLevels, ", "))
}

// WorkloadLogEntry is one log line. Timestamp is the server's raw string
// (not RFC3339); it is displayed verbatim and parsed best-effort for the
// follow cursor.
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

// GetWorkloadLogs returns up to limit of the most recent log lines,
// oldest-first for display (like `kubectl logs --tail`). Empty level keeps
// the server default.
func GetWorkloadLogs(workloadID string, limit int, level string) ([]WorkloadLogEntry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	all, err := fetchWorkloadLogs(workloadID, limit, level, "", "workload logs")
	if err != nil {
		return nil, err
	}

	// Server returns newest first; reverse to chronological for display.
	slices.Reverse(all)

	return all, nil
}

// logsQueryParams assembles the limit, optional level, and optional startTime
// query params.
func logsQueryParams(maxEntries int, level, since string) url.Values {
	pageSize := maxLogsPageSize
	if maxEntries > 0 {
		pageSize = min(maxEntries, maxLogsPageSize)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(pageSize))

	if level != "" {
		query.Set("level", strings.ToLower(level))
	}

	if since != "" {
		query.Set("startTime", since)
	}

	return query
}

// appendUnseenPageEntries appends page entries, skipping keys seen on an
// earlier page (offset paging over a live stream can re-serve a shifted
// line). Same-page duplicates are kept.
func appendUnseenPageEntries(all, page []WorkloadLogEntry, priorPages map[string]struct{}) []WorkloadLogEntry {
	for _, e := range page {
		if _, ok := priorPages[logKey(e)]; !ok {
			all = append(all, e)
		}
	}

	for _, e := range page {
		priorPages[logKey(e)] = struct{}{}
	}

	return all
}

// fetchWorkloadLogs retrieves log lines newest-first across pages. since (if
// set) is the startTime filter; maxEntries <= 0 drains every page. An empty
// page stops the loop even if a next link is present. reqInfo is drapi's
// per-request log label; the follow loop passes "" to silence the per-poll
// "Fetching ..." line so it does not interleave with the streamed log lines.
func fetchWorkloadLogs(workloadID string, maxEntries int, level, since, reqInfo string) ([]WorkloadLogEntry, error) {
	pageURL, err := drapi.EndpointURL("/otel/workload/"+escapeID(workloadID)+"/logs/", logsQueryParams(maxEntries, level, since))
	if err != nil {
		return nil, err
	}

	var all []WorkloadLogEntry

	priorPages := make(map[string]struct{})

	for pageURL != "" {
		var resp workloadLogsResponse

		if err := drapi.GetJSON(pageURL, reqInfo, &resp); err != nil {
			return nil, err
		}

		if len(resp.Data) == 0 {
			break
		}

		all = appendUnseenPageEntries(all, resp.Data, priorPages)

		if maxEntries > 0 && len(all) >= maxEntries {
			all = all[:maxEntries]

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

	return all, nil
}

// FollowWorkloadLogs streams a workload's log lines until ctx is cancelled
// (Ctrl-C ends it cleanly with nil) or a terminal fetch error occurs. It
// seeds with the most recent limit lines, then polls for newer ones, falling
// back to a re-fetched window when timestamps are unparseable or the
// startTime filter is rejected.
//
// onLine receives each new line in chronological order (a non-nil return
// ends the follow); onWarn (nil-safe) receives non-fatal conditions.
// Transient failures retry up to maxTransientPollErrors; others are terminal.
func FollowWorkloadLogs(
	ctx context.Context,
	workloadID string,
	limit int,
	level string,
	interval time.Duration,
	onLine func(WorkloadLogEntry) error,
	onWarn func(string),
) error {
	f, err := newLogFollower(workloadID, limit, level, interval, onLine, onWarn)
	if err != nil {
		return err
	}

	for {
		entries, hadSince, err := f.fetch() //nolint:contextcheck // drapi does not yet accept context; ctx gates the inter-poll sleeps
		if err != nil {
			retryNow, ferr := f.fetchFailure(err, hadSince)
			if ferr != nil {
				return ferr
			}

			if retryNow {
				continue
			}

			if !sleepInterval(ctx, interval) {
				return nil
			}

			continue
		}

		if err := f.emit(entries, hadSince); err != nil {
			return err
		}

		if !sleepInterval(ctx, interval) {
			return nil
		}
	}
}

// logFollower holds one follow stream's state between polls.
type logFollower struct {
	workloadID string
	limit      int
	level      string
	onLine     func(WorkloadLogEntry) error
	onWarn     func(string)

	dedup           *logDedup
	cursor          time.Time // newest parsed timestamp; zero means window mode
	cursorUsable    bool      // false once the server rejects the startTime filter
	seeded          bool
	transientErrors int
}

func newLogFollower(
	workloadID string,
	limit int,
	level string,
	interval time.Duration,
	onLine func(WorkloadLogEntry) error,
	onWarn func(string),
) (*logFollower, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	if interval <= 0 {
		return nil, fmt.Errorf("invalid interval %s: must be positive", interval)
	}

	if onLine == nil {
		return nil, errors.New("onLine callback is required")
	}

	if onWarn == nil {
		onWarn = func(string) {}
	}

	return &logFollower{
		workloadID:   workloadID,
		limit:        limit,
		level:        level,
		onLine:       onLine,
		onWarn:       onWarn,
		dedup:        newLogDedup(followSeenCap),
		cursorUsable: true,
	}, nil
}

// fetch retrieves the next poll's entries: the newest-limit window when
// seeding or in window mode, else everything newer than the cursor. hadSince
// reports which mode was used.
func (f *logFollower) fetch() (entries []WorkloadLogEntry, hadSince bool, err error) {
	since := ""
	maxEntries := f.limit

	if f.seeded && f.cursorUsable && !f.cursor.IsZero() {
		since = f.cursor.Add(-followLagAllowance).UTC().Format(time.RFC3339Nano)
		maxEntries = 0
	}

	// Empty reqInfo silences drapi's per-request "Fetching ..." log so the
	// follow stream stays just the workload's log lines.
	entries, err = fetchWorkloadLogs(f.workloadID, maxEntries, f.level, since, "")

	return entries, since != "", err
}

// fetchFailure decides how a failed poll continues: a rejected time filter
// drops to window mode and retries now (retryNow); an isolated transient is
// slept over; anything else ends the follow.
func (f *logFollower) fetchFailure(err error, hadSince bool) (retryNow bool, _ error) {
	if hadSince && isFilterRejectedError(err) {
		f.cursorUsable = false

		f.onWarn(fmt.Sprintf("server rejected the time filter, following the most recent %d lines per poll instead: %v", f.limit, err))

		return true, nil
	}

	if !isTransientPollError(err) {
		return false, err
	}

	f.transientErrors++

	if f.transientErrors > maxTransientPollErrors {
		return false, fmt.Errorf("fetch workload logs: %d consecutive transient errors, last: %w", f.transientErrors, err)
	}

	f.onWarn(fmt.Sprintf("transient error fetching logs, retrying: %v", err))

	return false, nil
}

// emit prints the poll's unseen entries in chronological order and advances
// the cursor past the newest parseable timestamp.
func (f *logFollower) emit(entries []WorkloadLogEntry, hadSince bool) error {
	f.transientErrors = 0

	// Newest first from the server; chronological for display.
	slices.Reverse(entries)

	fresh := f.dedup.filterUnseen(entries)

	// A full window with zero overlap means >limit lines arrived since the
	// last poll and the excess is unfetchable.
	if f.seeded && !hadSince && len(entries) == f.limit && len(fresh) == len(entries) {
		f.onWarn(fmt.Sprintf("possible gap: more than %d new lines arrived since the last poll and some may have been skipped (re-run with a larger --limit)", f.limit))
	}

	for _, e := range fresh {
		if err := f.onLine(e); err != nil {
			return err
		}
	}

	for _, e := range entries {
		if t, ok := parseLogTimestamp(e.Timestamp); ok && t.After(f.cursor) {
			f.cursor = t
		}
	}

	f.seeded = true

	return nil
}

// isFilterRejectedError reports whether the server rejected the query params
// outright (400/422).
func isFilterRejectedError(err error) bool {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusBadRequest || httpErr.StatusCode == http.StatusUnprocessableEntity
	}

	return false
}

// logTimestampLayouts are the timestamp shapes the gateway emits: Python's
// str(datetime), with RFC3339 as a fallback.
var logTimestampLayouts = []string{
	"2006-01-02 15:04:05.999999999Z07:00",
	time.RFC3339Nano,
}

// parseLogTimestamp parses a server timestamp, reporting false when the
// shape is unrecognized (the follow then stays in window mode).
func parseLogTimestamp(value string) (time.Time, bool) {
	for _, layout := range logTimestampLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

// logKey identifies a log line for dedup. Timestamp alone is not unique, so
// level and message are included; truly identical same-instant lines still
// collide (no per-line ID exists).
func logKey(e WorkloadLogEntry) string {
	return e.Timestamp + "\x00" + e.Level + "\x00" + e.Message
}

// logDedup tracks emitted log lines, bounded by generational rotation: a
// full current generation becomes prev (both consulted), so the last
// genCap..2*genCap lines stay deduplicated.
type logDedup struct {
	cur, prev map[string]struct{}
	genCap    int
}

func newLogDedup(genCap int) *logDedup {
	return &logDedup{cur: make(map[string]struct{}), genCap: genCap}
}

// filterUnseen returns the entries whose key has not been seen, recording
// them. Input order is preserved (the caller passes chronological entries).
func (d *logDedup) filterUnseen(entries []WorkloadLogEntry) []WorkloadLogEntry {
	fresh := make([]WorkloadLogEntry, 0, len(entries))

	for _, e := range entries {
		key := logKey(e)

		if _, ok := d.cur[key]; ok {
			continue
		}

		if _, ok := d.prev[key]; ok {
			continue
		}

		if len(d.cur) >= d.genCap {
			d.prev = d.cur
			d.cur = make(map[string]struct{}, d.genCap)
		}

		d.cur[key] = struct{}{}

		fresh = append(fresh, e)
	}

	return fresh
}
