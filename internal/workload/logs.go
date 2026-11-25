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

// followSeenCap bounds each follow dedup generation. It is a memory ceiling,
// not the duplicate-suppression horizon: a since-mode follower prunes keys
// that leave its fetch overlap, so live keys stay near the overlap's own
// size and rotation never fires below a sustained ~800 lines/s. Only window
// mode, which has no cursor to prune by, leans on the rotation itself.
const followSeenCap = 50000

// maxTransientPollErrors caps consecutive transient fetch failures a poll loop
// tolerates before giving up; it resets on any successful poll. Shared by the
// log follow and by pollWorkload, which want the same answer for the same
// reason: both run long enough for a blip to land mid-wait.
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

// LogFilter narrows a log fetch. The zero value is no filter.
//
// The route takes what it takes and no more, measured against it rather than
// assumed: a level, a single case-insensitive substring search
// on the message (searchKeys/searchValues), a trace id, a span id, and a time
// window in RFC 3339 with a Z suffix. It refuses unknown parameters with a
// 400 and has no exclusion parameter at all. So the first search term and
// the ids and the window go to the server, and everything the server cannot
// do is done here after the fetch: the second and later search terms, and
// every exclusion.
type LogFilter struct {
	// Level is the minimum severity, already validated by ParseLogLevel.
	Level string

	// Grep is the substrings a line must contain, every one of them, matched
	// without regard to case. The first narrows the fetch server-side; all
	// of them are checked here, so a line is never shown on the strength of
	// the server's reading alone.
	Grep []string

	// Exclude is the substrings a line must not contain, matched without
	// regard to case. Client-side only.
	Exclude []string

	// TraceID and SpanID select the lines of one trace or span.
	TraceID string
	SpanID  string

	// Since and Until bound the window. Zero means unbounded on that side.
	Since time.Time
	Until time.Time
}

// keep returns the entries the client-side half of the filter admits, in
// the order given. A filter with nothing client-side hands the slice back.
func (f LogFilter) keep(entries []WorkloadLogEntry) []WorkloadLogEntry {
	if len(f.Grep) == 0 && len(f.Exclude) == 0 {
		return entries
	}

	kept := entries[:0:0]

	for _, e := range entries {
		if f.admits(e.Message) {
			kept = append(kept, e)
		}
	}

	return kept
}

func (f LogFilter) admits(message string) bool {
	lower := strings.ToLower(message)

	for _, term := range f.Grep {
		if !strings.Contains(lower, strings.ToLower(term)) {
			return false
		}
	}

	for _, term := range f.Exclude {
		if strings.Contains(lower, strings.ToLower(term)) {
			return false
		}
	}

	return true
}

// logTimeFormat is the one shape the route accepts for a bound: RFC 3339 in
// UTC with the Z suffix. The same instant spelled with a +00:00 offset is
// refused as an invalid date string, so a bound is always converted to UTC
// before it is formatted.
func logTimeFormat(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// logDateLayout is a bare day, the one form whose meaning depends on which
// end of the window it bounds.
const logDateLayout = "2006-01-02"

// logTimeLayouts are the absolute forms ParseLogTime reads, most specific
// first: the RFC 3339 the route speaks, the shape the command itself prints
// on every line (so a printed timestamp can be pasted straight back into
// --since or --until), and the shorter spellings a person types. A form
// without a zone is read as UTC, which is what the printed timestamps carry.
var logTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	logDateLayout,
}

// Narrows reports whether the filter drops anything at all, so an empty
// result can be told apart from a workload with no logs.
func (f LogFilter) Narrows() bool {
	return f.Level != "" || len(f.Grep) > 0 || len(f.Exclude) > 0 ||
		f.TraceID != "" || f.SpanID != "" || !f.Since.IsZero() || !f.Until.IsZero()
}

// ParseLogTime reads a --since or --until value: an absolute time in RFC 3339
// or a date, or a duration back from now such as 15m, 2h30m, 1d or 1w. Go's
// own duration syntax stops at hours, and "yesterday" is what a person means
// by 1d, so days and weeks are read here and turned into hours.
func ParseLogTime(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}

	for _, layout := range logTimeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}

	if d, ok := parseRelativeDuration(value); ok {
		return now.Add(-d), nil
	}

	return time.Time{}, fmt.Errorf(
		"invalid time %q: use RFC 3339 (2026-06-11T14:04:15Z), a date (2026-06-11), or a duration back from now (15m, 2h, 1d, 1w)",
		value)
}

// ParseLogUntil is ParseLogTime for the closing end of a window, where a
// bare date means the whole of that day: --until 2026-06-11 keeps what was
// logged on the 11th, rather than ending the window as the day began.
func ParseLogUntil(value string, now time.Time) (time.Time, error) {
	t, err := ParseLogTime(value, now)
	if err != nil {
		return t, err
	}

	if _, isDate := time.Parse(logDateLayout, strings.TrimSpace(value)); isDate == nil {
		return t.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	}

	return t, nil
}

// parseRelativeDuration reads a positive duration, accepting Go's forms plus
// a trailing d or w for days and weeks.
func parseRelativeDuration(value string) (time.Duration, bool) {
	if d, err := time.ParseDuration(value); err == nil {
		return d, d > 0
	}

	unit := time.Duration(0)

	switch {
	case strings.HasSuffix(value, "d"):
		unit = 24 * time.Hour
	case strings.HasSuffix(value, "w"):
		unit = 7 * 24 * time.Hour
	default:
		return 0, false
	}

	n, err := strconv.ParseFloat(strings.TrimSuffix(value, value[len(value)-1:]), 64)
	if err != nil || n <= 0 {
		return 0, false
	}

	return time.Duration(n * float64(unit)), true
}

// GetWorkloadLogs returns up to limit of the most recent log lines that pass
// filter, oldest-first for display (like `kubectl logs --tail`).
//
// limit bounds what is fetched, before the client-side half of the filter
// runs, so a fetch with exclusions or several search terms can return fewer
// lines than limit. Bounding the result instead would mean fetching pages
// until enough survived, with no upper bound on how far back that reaches.
func GetWorkloadLogs(workloadID string, limit int, filter LogFilter) ([]WorkloadLogEntry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	all, err := fetchWorkloadLogs(workloadID, limit, filter, "", "workload logs")
	if err != nil {
		return nil, err
	}

	all = filter.keep(all)

	// Server returns newest first; reverse to chronological for display.
	slices.Reverse(all)

	return all, nil
}

// logsQueryParams assembles the query: the page size, the server-side half of
// the filter, and the window's start. cursor, when set, is a follow's own
// start time and takes the place of the filter's Since: the follow has
// already shown everything before it.
func logsQueryParams(maxEntries int, filter LogFilter, cursor string) url.Values {
	pageSize := maxLogsPageSize
	if maxEntries > 0 {
		pageSize = min(maxEntries, maxLogsPageSize)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(pageSize))

	if filter.Level != "" {
		query.Set("level", strings.ToLower(filter.Level))
	}

	if len(filter.Grep) > 0 {
		query.Set("searchKeys", "message")
		query.Set("searchValues", filter.Grep[0])
	}

	if filter.TraceID != "" {
		query.Set("traceId", filter.TraceID)
	}

	if filter.SpanID != "" {
		query.Set("spanId", filter.SpanID)
	}

	switch {
	case cursor != "":
		query.Set("startTime", cursor)
	case !filter.Since.IsZero():
		query.Set("startTime", logTimeFormat(filter.Since))
	}

	if !filter.Until.IsZero() {
		query.Set("endTime", logTimeFormat(filter.Until))
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

// fetchWorkloadLogs retrieves log lines newest-first across pages, as the
// server filtered them. cursor (if set) is a follow's startTime and overrides
// the filter's own Since; maxEntries <= 0 drains every page. An empty page
// stops the loop even if a next link is present. reqInfo is drapi's
// per-request log label; the follow loop passes "" to silence the per-poll
// "Fetching ..." line so it does not interleave with the streamed log lines.
//
// The client-side half of the filter is not applied here on purpose: the
// follower needs the unfiltered batch for its cursor and its gap arithmetic,
// and applies the filter as it prints.
func fetchWorkloadLogs(workloadID string, maxEntries int, filter LogFilter, cursor, reqInfo string) ([]WorkloadLogEntry, error) {
	pageURL, err := drapi.EndpointURL("/otel/workload/"+escapeID(workloadID)+"/logs/", logsQueryParams(maxEntries, filter, cursor))
	if err != nil {
		return nil, err
	}

	return drainLogPages(pageURL, maxEntries, reqInfo)
}

// drainLogPages walks an OTEL logs route's pagination from pageURL,
// newest-first, applying the same page-overlap dedup and maxEntries cap for
// every log source.
func drainLogPages(pageURL string, maxEntries int, reqInfo string) ([]WorkloadLogEntry, error) {
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

// logsFetchFn retrieves one poll's worth of log lines newest-first. It is the
// seam that lets the follower stream any OTEL log source: the workload stream
// and a build's filtered artifact stream differ only in this function.
type logsFetchFn func(maxEntries int, level, since, reqInfo string) ([]WorkloadLogEntry, error)

// FollowWorkloadLogs streams a workload's log lines until ctx is cancelled
// (Ctrl-C ends it cleanly with nil) or a terminal fetch error occurs. It
// seeds with the most recent limit lines, then polls for newer ones, falling
// back to a re-fetched window when timestamps are unparseable or the
// startTime filter is rejected.
//
// filter applies throughout: the server-side half on every poll, the
// client-side half as lines are printed. Since bounds the seed only, since
// every later poll starts from the cursor; Until is refused, because a
// follow is by definition open-ended.
//
// onLine receives each new line in chronological order (a non-nil return
// ends the follow); onWarn (nil-safe) receives non-fatal conditions.
// Transient failures retry up to maxTransientPollErrors; others are terminal.
func FollowWorkloadLogs(
	ctx context.Context,
	workloadID string,
	limit int,
	filter LogFilter,
	interval time.Duration,
	onLine func(WorkloadLogEntry) error,
	onWarn func(string),
) error {
	if !filter.Until.IsZero() {
		return errors.New("an end time cannot be combined with following: a follow has no end")
	}

	fetch := func(maxEntries int, _, cursor, reqInfo string) ([]WorkloadLogEntry, error) { //nolint:contextcheck // drapi does not yet accept context; ctx gates the inter-poll sleeps
		return fetchWorkloadLogs(workloadID, maxEntries, filter, cursor, reqInfo)
	}

	f, err := newLogFollower(fetch, limit, filter.Level, interval, onLine, onWarn)
	if err != nil {
		return err
	}

	f.gapHint = " (re-run with a larger --limit)"
	f.keep = filter.keep
	f.since = filter.Since

	for {
		entries, hadSince, err := f.fetch()
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
	fetchFn logsFetchFn
	limit   int
	level   string
	onLine  func(WorkloadLogEntry) error
	onWarn  func(string)

	// gapHint is appended to the possible-gap warning; it names the remedy,
	// which only the caller knows (a --limit flag exists on `workload logs`
	// but not on every stream this follower serves).
	gapHint string

	// lag is how far behind the newest-seen timestamp the cursor trails, so
	// late-ingested lines are caught by the dedup overlap rather than
	// skipped. Callers whose source ingests slowly (build logs) widen it.
	lag time.Duration

	// since is the window's start, when the caller gave one. The cursor
	// never reaches back before it: the lag allowance would otherwise pull
	// in lines older than the window on the poll after a seed whose newest
	// line sits within the allowance of the start. Zero means no floor.
	since time.Time

	// keep is the client-side filter, applied to the lines about to be
	// printed and to nothing else: the cursor, the dedup and the gap check
	// all work on the batch as the server returned it, so a line filtered
	// out here still moves the cursor and still counts toward a full window.
	// nil keeps everything.
	keep func([]WorkloadLogEntry) []WorkloadLogEntry

	dedup           *logDedup
	cursor          time.Time // newest parsed timestamp; zero means window mode
	cursorUsable    bool      // false once the server rejects the startTime filter
	seeded          bool
	transientErrors int
}

func newLogFollower(
	fetchFn logsFetchFn,
	limit int,
	level string,
	interval time.Duration,
	onLine func(WorkloadLogEntry) error,
	onWarn func(string),
) (*logFollower, error) {
	if fetchFn == nil {
		return nil, errors.New("fetchFn is required")
	}

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
		fetchFn:      fetchFn,
		limit:        limit,
		level:        level,
		onLine:       onLine,
		onWarn:       onWarn,
		dedup:        newLogDedup(followSeenCap),
		cursorUsable: true,
		lag:          followLagAllowance,
	}, nil
}

// fetch retrieves the next poll's entries: the newest-limit window when
// seeding or in window mode, else everything newer than the cursor. hadSince
// reports which mode was used.
func (f *logFollower) fetch() (entries []WorkloadLogEntry, hadSince bool, err error) {
	since := ""
	maxEntries := f.limit

	if f.seeded && f.cursorUsable && !f.cursor.IsZero() {
		start := f.cursor.Add(-f.lag)
		if start.Before(f.since) {
			start = f.since
		}

		since = start.UTC().Format(time.RFC3339Nano)
		maxEntries = 0
	}

	// Empty reqInfo silences drapi's per-request "Fetching ..." log so the
	// follow stream stays just the source's log lines.
	entries, err = f.fetchFn(maxEntries, f.level, since, "")

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
		return false, fmt.Errorf("fetch logs: %d consecutive transient errors, last: %w", f.transientErrors, err)
	}

	f.onWarn(fmt.Sprintf("transient error fetching logs, retrying: %v", err))

	return false, nil
}

// emit prints the poll's unseen entries in chronological order and advances
// the cursor past the newest parseable timestamp.
func (f *logFollower) emit(entries []WorkloadLogEntry, hadSince bool) error {
	f.transientErrors = 0

	// Newest first from the server; chronological for display. The reverse
	// gets the bulk order right; the sort settles lines the collector
	// ingested out of event order.
	slices.Reverse(entries)
	sortChronological(entries)

	fresh := f.dedup.filterUnseen(entries)

	// A full window with zero overlap means >limit lines arrived since the
	// last poll and the excess is unfetchable.
	if f.seeded && !hadSince && len(entries) == f.limit && len(fresh) == len(entries) {
		f.onWarn(fmt.Sprintf("possible gap: more than %d new lines arrived since the last poll and some may have been skipped%s", f.limit, f.gapHint))
	}

	if err := f.print(fresh); err != nil {
		return err
	}

	f.advanceCursor(entries)

	// Forget only what the next fetch can no longer return: a since fetch
	// starts at cursor-lag, so a key older than that is never offered again,
	// while one inside the window must stay known however many lines came
	// after it. Without this the dedup's rotation is a horizon a fast
	// builder outruns, and the overlap prints twice.
	if f.cursorUsable && !f.cursor.IsZero() {
		f.dedup.prune(f.cursor.Add(-f.lag))
	}

	// Seeding waits for the first non-empty batch — not for the cursor's
	// sake (an empty fetch leaves it zero, and the next poll is a window
	// fetch either way) but for the gap warning above, which reads f.seeded
	// as "the seed batch has already been shown". Armed by a leading empty
	// poll, the first real batch filling the window would read as lines
	// lost, when it is just the seed being as big as it was asked to be.
	if len(entries) > 0 {
		f.seeded = true
	}

	return nil
}

// print hands the unseen lines to the caller, the client-side filter applied
// on the way: this is the one place it runs, so the cursor and the gap check
// keep seeing the batch as the server returned it.
func (f *logFollower) print(fresh []WorkloadLogEntry) error {
	if f.keep != nil {
		fresh = f.keep(fresh)
	}

	for _, e := range fresh {
		if err := f.onLine(e); err != nil {
			return err
		}
	}

	return nil
}

// sortChronological stable-sorts entries by their parsed timestamps, settling
// lines the collector ingested out of event order (a build's "#1 DONE"
// arriving before its "#1 [internal] load"). Unparseable timestamps stay
// where the server put them.
func sortChronological(entries []WorkloadLogEntry) {
	slices.SortStableFunc(entries, func(a, b WorkloadLogEntry) int {
		ta, aok := parseLogTimestamp(a.Timestamp)
		tb, bok := parseLogTimestamp(b.Timestamp)

		if !aok || !bok {
			return 0
		}

		return ta.Compare(tb)
	})
}

// advanceCursor moves the cursor past the newest parseable timestamp.
func (f *logFollower) advanceCursor(entries []WorkloadLogEntry) {
	for _, e := range entries {
		if t, ok := parseLogTimestamp(e.Timestamp); ok && t.After(f.cursor) {
			f.cursor = t
		}
	}
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

// logDedup tracks emitted log lines. Two bounds keep it honest. The caller
// prunes keys older than what its next fetch can re-offer — the exact
// invariant, because a key inside the fetch overlap has to stay known however
// many lines arrived after it, or a fast builder's lines print twice off-TTY
// where every duplicate is permanent. Generational rotation stays as the
// memory ceiling for the callers where nothing prunes (window mode has no
// cursor to prune by): the last genCap..2*genCap lines stay deduplicated.
type logDedup struct {
	cur, prev map[string]time.Time
	genCap    int

	// watermark is the newest parseable event time seen. A line whose own
	// timestamp does not parse adopts it, so it ages out with its neighbors
	// rather than living forever or dying at once.
	watermark time.Time
}

func newLogDedup(genCap int) *logDedup {
	return &logDedup{cur: make(map[string]time.Time), genCap: genCap}
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
			d.cur = make(map[string]time.Time, d.genCap)
		}

		at, ok := parseLogTimestamp(e.Timestamp)
		if ok && at.After(d.watermark) {
			d.watermark = at
		} else if !ok {
			at = d.watermark
		}

		d.cur[key] = at

		fresh = append(fresh, e)
	}

	return fresh
}

// prune forgets the keys no future fetch can re-offer: everything strictly
// older than before, which the caller sets to its fetch floor (the cursor
// minus its lag). Pruning is what keeps the generation cap a memory ceiling
// rather than a duplicate-suppression horizon a fast producer can outrun:
// kept ahead of the rotation, the live keys never reach genCap in the first
// place.
func (d *logDedup) prune(before time.Time) {
	for _, generation := range []map[string]time.Time{d.cur, d.prev} {
		for key, at := range generation {
			if at.Before(before) {
				delete(generation, key)
			}
		}
	}
}
