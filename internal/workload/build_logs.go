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
	"slices"
	"time"

	"github.com/datarobot/cli/internal/drapi"
)

// A build's logs live on the artifact-wide OTEL route
// /api/v2/otel/artifact/{id}/logs/: an artifact accumulates every build it
// ever ran, and the platform's own build id is internal, so searchKeys/
// searchValues on external_build_id — the build id this CLI already holds
// from the trigger response — is the only way to narrow the stream to one
// build. The query keys are camelCase; their snake_case forms are rejected
// with a 400.

// buildLogSeedLimit is how many lines a window-mode fetch may carry: the
// server's own page cap, because a builder can burst hundreds of lines in
// one poll interval and anything smaller gaps the opening of the build.
// Once the first non-empty batch seeds the time cursor, fetches are
// since-based and unbounded, so this only governs the first batch and an
// attach's seed.
const buildLogSeedLimit = maxLogsPageSize

// buildLogLagAllowance is the build tail's cursor lag. Build-log ingestion
// trails the builder by 20-40s (measured on staging), far past the runtime
// follower's default; a line ingested later than the lag window is skipped
// forever, so the window errs wide and the dedup absorbs the overlap.
const buildLogLagAllowance = 60 * time.Second

// fetchArtifactBuildLogs retrieves one build's log lines newest-first across
// pages, by filtering the artifact's OTEL stream on external_build_id.
func fetchArtifactBuildLogs(artifactID, buildID string, maxEntries int, level, since, reqInfo string) ([]WorkloadLogEntry, error) {
	query := logsQueryParams(maxEntries, level, since)
	query.Set("searchKeys", "external_build_id")
	query.Set("searchValues", buildID)

	pageURL, err := drapi.EndpointURL("/otel/artifact/"+escapeID(artifactID)+"/logs/", query)
	if err != nil {
		return nil, err
	}

	return drainLogPages(pageURL, maxEntries, reqInfo)
}

// buildLogHoldback is how long a line waits in the reorder buffer, measured
// against the newest event time seen. OTEL ingestion delivers lines seconds
// out of order across polls; holding each line back lets a late sibling slot
// in ahead of it, at the cost of the stream trailing the builder by this
// much. Finish drains the buffer regardless, so nothing is ever withheld
// past the build's end.
const buildLogHoldback = 10 * time.Second

// BuildLogTail streams one build's log lines incrementally, driven by the
// caller's own cadence (WaitForBuild's per-poll callback) rather than a
// clock of its own. Lines pass through a holdback reorder buffer so
// late-ingested lines print in event order rather than arrival order.
//
// It is strictly progress feedback: no fetch failure is ever surfaced as an
// error, because a build must not fail — or appear to — over a hiccup in
// reading its logs. After too many consecutive failures the tail disables
// itself, says so once through onWarn, and stays quiet.
type BuildLogTail struct {
	follower *logFollower
	onLine   func(WorkloadLogEntry)
	onWarn   func(string)
	disabled bool

	// The reorder buffer: lines sorted by event time (arrival order breaking
	// ties), flushed once they age past the newest-seen event time minus the
	// holdback.
	pending []bufferedLogLine
	seq     int
	newest  time.Time
}

// bufferedLogLine is one held-back line: its parsed event time and an
// arrival sequence number that keeps equal or unparseable timestamps stable.
type bufferedLogLine struct {
	at    time.Time
	seq   int
	entry WorkloadLogEntry
}

// NewBuildLogTail builds a tail for one build's lines. onLine receives each
// unseen line in chronological order; onWarn (nil-safe) receives the one
// notice given when the tail gives up.
func NewBuildLogTail(artifactID, buildID string, onLine func(WorkloadLogEntry), onWarn func(string)) *BuildLogTail {
	if onWarn == nil {
		onWarn = func(string) {}
	}

	fetch := func(maxEntries int, level, since, reqInfo string) ([]WorkloadLogEntry, error) {
		return fetchArtifactBuildLogs(artifactID, buildID, maxEntries, level, since, reqInfo)
	}

	// The interval passed here only satisfies the follower's validation; the
	// caller's poll cadence is the real clock. Level stays the server default
	// (debug) deliberately: dim noise costs a glance, a failure detail the
	// builder only said at debug costs a debugging session.
	tail := &BuildLogTail{onLine: onLine, onWarn: onWarn}

	follower, err := newLogFollower(fetch, buildLogSeedLimit, "", time.Second,
		func(e WorkloadLogEntry) error { tail.buffer(e); return nil }, onWarn)
	if err != nil {
		// Unreachable with the constants above; a nil follower simply means
		// the tail never emits, which is the contract's worst case anyway.
		tail.disabled = true

		return tail
	}

	follower.lag = buildLogLagAllowance
	tail.follower = follower

	return tail
}

// buffer inserts one line into the reorder buffer and advances the
// event-time watermark. Unparseable timestamps adopt the current watermark,
// so they flush with their neighbors instead of jamming the buffer.
func (t *BuildLogTail) buffer(e WorkloadLogEntry) {
	at, ok := parseLogTimestamp(e.Timestamp)
	if !ok {
		at = t.newest
	}

	if at.After(t.newest) {
		t.newest = at
	}

	t.pending = append(t.pending, bufferedLogLine{at: at, seq: t.seq, entry: e})
	t.seq++
}

// flush emits buffered lines in event order: everything when all is set,
// otherwise only lines older than the holdback watermark, so a late sibling
// still to arrive can slot in ahead of what remains held.
func (t *BuildLogTail) flush(all bool) {
	slices.SortStableFunc(t.pending, func(a, b bufferedLogLine) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}

		return a.seq - b.seq
	})

	watermark := t.newest.Add(-buildLogHoldback)

	kept := t.pending[:0]

	for _, line := range t.pending {
		if all || !line.at.After(watermark) {
			t.onLine(line.entry)
		} else {
			kept = append(kept, line)
		}
	}

	t.pending = kept
}

// Finish performs the final catch-up poll and drains the reorder buffer.
// Call it once after the build wait ends: ingestion lags the build, so the
// last lines routinely land after the terminal status, and nothing may stay
// withheld once the stream is over.
func (t *BuildLogTail) Finish() {
	t.Poll()
	t.flush(true)
}

// Poll fetches once and emits any unseen lines. Call it from the build wait's
// per-poll callback, and once more after the wait ends to catch lines
// ingested between the terminal status and the last poll.
func (t *BuildLogTail) Poll() {
	if t.disabled {
		return
	}

	entries, hadSince, err := t.follower.fetch()
	if err != nil {
		retryNow, ferr := t.follower.fetchFailure(err, hadSince)
		if ferr != nil {
			t.disabled = true
			t.onWarn("build log streaming stopped; the build itself is unaffected: " + ferr.Error())

			return
		}

		if retryNow {
			t.Poll() // the follower dropped to window mode; re-fetch now

			return
		}

		return
	}

	// buffer never returns an error (see NewBuildLogTail), so emit cannot
	// fail; the return is the follower's contract, not this tail's.
	_ = t.follower.emit(entries, hadSince)

	t.flush(false)
}
