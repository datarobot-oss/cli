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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Replacement statuses observed to be terminal. The platform's own docs name
// only "completed" and "failed"; live testing against staging also produced
// "errored", a candidate that never became healthy, which the platform then
// clears with a 404 like any other settled replacement. Without treating
// "errored" as a failure here, that 404 would read as a quiet success (see
// WaitForReplacement).
//
// The non-terminal vocabulary is not documented anywhere, so
// IsTerminalReplacementStatus treats an unrecognised status as still in
// progress rather than guessing, the same stance IsTerminalBuildStatus takes.
//
// Known limitation, and the reason that stance is not free. A failure status
// the platform adds later would be classified as still in progress, and if
// the record is then cleared, WaitForReplacement reports the rollout as a
// success. That "errored" had to be added at all is proof the vocabulary was
// incomplete once already. Closing the hole properly means confirming after
// the fact which artifact the workload ended up running, which needs a
// verified answer about how quickly the workload reflects a completed
// replacement; guessing at that would trade a rare false success for a
// routine false failure. Worth settling the first time a rollout is driven
// against a live instance.
const (
	ReplacementStatusCompleted = "completed"
	ReplacementStatusFailed    = "failed"
	ReplacementStatusErrored   = "errored"
)

// rollingStrategy is the only strategy the platform ships. Blue-green and
// canary are on its roadmap, at which point this becomes a parameter.
const rollingStrategy = "rolling"

// Replacement is the resource behind GET, POST and DELETE on
// /workloads/{id}/replacement/. The shape was confirmed live against staging:
// the target artifact comes back as candidateArtifactId, not as artifactId,
// which is what the platform's own example code implies. ArtifactID and
// Status carry the polling and rollout logic; the rest are display fields.
type Replacement struct {
	ID         string    `json:"id"`
	WorkloadID string    `json:"workloadId"`
	ArtifactID string    `json:"candidateArtifactId"`
	Status     string    `json:"status"`
	Strategy   string    `json:"strategy,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// IsTerminalReplacementStatus reports whether s is a status the replacement
// will not progress from.
func IsTerminalReplacementStatus(s string) bool {
	switch s {
	case ReplacementStatusCompleted, ReplacementStatusFailed, ReplacementStatusErrored:
		return true
	}

	return false
}

// IsFailedReplacementStatus reports whether s is a terminal failure. On
// failure the workload reverts to the artifact it was running before the
// replacement started, so a failed rollout never promotes.
func IsFailedReplacementStatus(s string) bool {
	return s == ReplacementStatusFailed || s == ReplacementStatusErrored
}

// replacementURL builds the single route all three verbs share.
func replacementURL(workloadID string) (string, error) {
	return config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/replacement/")
}

// GetActiveReplacement fetches the in-flight replacement for workloadID, if
// there is one. The server answers 404 when none is active, which is the
// expected "nothing to guard against" answer rather than an error, so it
// becomes (nil, nil).
//
// That 404 is ambiguous and this function does not resolve it: the route
// answers the same way for a workload that does not exist, and the body
// saying which never reaches this layer, because drapi.Get discards it. A
// caller that has not already established the workload exists should use
// GuardNoActiveReplacement, which pays for one extra request to tell the two
// apart, rather than reading (nil, nil) as proof of anything.
func GetActiveReplacement(workloadID string) (*Replacement, error) {
	url, err := replacementURL(workloadID)
	if err != nil {
		return nil, err
	}

	var replacement Replacement

	if err := drapi.GetJSON(url, "replacement", &replacement); err != nil {
		if isNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	return &replacement, nil
}

// StartReplacement rolls workloadID's running artifact onto artifactID and
// returns the created replacement, which should be handed to
// WaitForReplacement so the wait knows a rollout really was started.
//
// It is not idempotent: called while a replacement is already in flight it
// queues a second swap rather than rejecting, so callers guard with
// GuardNoActiveReplacement first.
//
// Both artifacts must share draft or locked status; the platform rejects a
// mismatch, which is why a locked workload has to have its successor locked
// before this is called. That is not enforced here because the caller is the
// one holding both statuses, and the lock has to happen between the guard and
// this call.
func StartReplacement(workloadID, artifactID string) (*Replacement, error) {
	url, err := replacementURL(workloadID)
	if err != nil {
		return nil, err
	}

	body := map[string]string{
		"artifactId": artifactID,
		"strategy":   rollingStrategy,
	}

	var replacement Replacement

	if err := drapi.PostJSON(url, "replacement", body, &replacement); err != nil {
		return nil, err
	}

	return &replacement, nil
}

// CancelReplacement tears down an in-flight candidate, leaving the previous
// version serving.
//
// A 404 means either that there was nothing to cancel or that the workload
// does not exist, and the route does not say which, so this checks. Nothing
// to cancel is success, since cancelling twice should be harmless and a
// rollout that settled on its own has already reached the state the caller
// wanted. A workload that does not exist is an error, because reporting a
// successful cancel for a mistyped id would leave the real rollout running
// while the user believes they stopped it.
//
// The response body is discarded, matching DeleteArtifact and DeleteWorkload.
// This route is documented but, unlike the rest of this file, has not been
// exercised against a live instance; if the platform answers 202 with a
// "cancelling" status rather than tearing down synchronously, the first
// caller will need to poll, and should confirm that on staging.
func CancelReplacement(workloadID string) error {
	url, err := replacementURL(workloadID)
	if err != nil {
		return err
	}

	if err := drapi.DeleteJSON(url, "replacement", nil, nil); err != nil {
		if isNotFound(err) {
			return confirmWorkloadExists(workloadID)
		}

		return err
	}

	return nil
}

// GuardNoActiveReplacement refuses to proceed when workloadID already has a
// replacement in flight, because POSTing another queues a second swap instead
// of erroring. Callers should run it twice: once up front, so a doomed
// command fails before it mutates anything, and once immediately before
// StartReplacement, which is the guard that actually holds, since the live
// state can change in between.
//
// Being the up-front check is why it does not simply trust an empty answer.
// The replacement route returns the same 404 for "nothing in flight" and for
// "no such workload", so an unchecked guard would wave through exactly the
// mistyped id it exists to catch, and the command would go on to create and
// possibly lock an artifact before discovering the target was never real.
// One extra request on the quiet path buys that, and the quiet path is about
// to spend minutes rolling a workload.
func GuardNoActiveReplacement(workloadID string) error {
	active, err := GetActiveReplacement(workloadID)
	if err != nil {
		return err
	}

	if active == nil {
		return confirmWorkloadExists(workloadID)
	}

	// A settled replacement stays readable for a while after it ends, and
	// WaitForReplacement returns the moment it sees a terminal status, so the
	// record is usually still there when the next command looks. Refusing on
	// it would block a retry, or a follow-up deploy, on a rollout that is
	// already over, with a message that contradicts itself by reporting the
	// status as "completed" while calling it in progress.
	if IsTerminalReplacementStatus(active.Status) {
		return nil
	}

	return fmt.Errorf(
		"workload %s already has a replacement in progress (to artifact %s, status %s); wait for it to settle before starting another",
		workloadID, active.ArtifactID, active.Status,
	)
}

// confirmWorkloadExists turns the replacement route's ambiguous 404 into a
// definite answer. The message stays factual; naming the remedy is the
// command layer's job, since only it knows whether the id came from a flag or
// from a manifest.
func confirmWorkloadExists(workloadID string) error {
	if _, err := GetWorkload(workloadID); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("workload %s does not exist or is not visible to this account", workloadID)
		}

		return fmt.Errorf("confirm workload %s exists: %w", workloadID, err)
	}

	return nil
}

// WaitForReplacement polls until the replacement reaches a terminal status,
// is cleared after having been seen, or timeout expires. started is what
// StartReplacement returned, and may be nil when attaching to a rollout this
// process did not begin. onTick, if non-nil, is called with every poll's
// result so a caller can show progress; it is never called with nil.
//
// It returns:
//
//   - the settled replacement and nil, when the rollout completed
//   - the settled replacement and an error, when it failed, so the caller can
//     still show what happened
//   - the last-seen replacement and an error, on timeout, for the same reason
//   - nil and an error, when a poll failed or nothing was ever active
//
// The two 404 cases mean opposite things, which is why this is not a plain
// poll loop. Seen after a status, a 404 means the platform settled the
// rollout and collected the record, which is success. Seen before any status,
// it means nothing was ever active. That second case used to be reported as
// an error unconditionally, which was wrong whenever the platform settled a
// fast rollout before the first poll landed: passing started lets the wait
// tell "it finished before I looked" from "it was never there".
func WaitForReplacement(
	workloadID string,
	started *Replacement,
	interval, timeout time.Duration,
	onTick func(*Replacement),
) (*Replacement, error) {
	if interval <= 0 {
		interval = defaultReplacementPollInterval
	}

	deadline := time.Now().Add(timeout)
	lastSeen := started

	for {
		replacement, err := GetActiveReplacement(workloadID)
		if err != nil {
			return nil, fmt.Errorf("poll replacement for workload %s: %w", workloadID, err)
		}

		if replacement == nil {
			if lastSeen == nil {
				// Only reachable when started was nil, since lastSeen begins
				// as started: this is the attach path, and the caller asked
				// to follow a rollout that is not running.
				return nil, fmt.Errorf("no replacement is in flight for workload %s", workloadID)
			}

			return lastSeen, nil
		}

		lastSeen = replacement

		if onTick != nil {
			onTick(replacement)
		}

		if IsTerminalReplacementStatus(replacement.Status) {
			return replacement, terminalReplacementErr(workloadID, replacement.Status)
		}

		if time.Now().After(deadline) {
			return replacement, fmt.Errorf("timeout waiting for replacement on workload %s after %s", workloadID, timeout)
		}

		time.Sleep(interval)
	}
}

// defaultReplacementPollInterval keeps an exported poller from busy-spinning
// when handed a non-positive interval. The CLI's own flags cannot produce one
// (pollflags rejects it), but nothing about this function's signature says so.
const defaultReplacementPollInterval = 2 * time.Second

// terminalReplacementErr turns a terminal status into the error the caller
// should return, or nil when the rollout succeeded. It is only meaningful for
// a status IsTerminalReplacementStatus accepts; anything else reads as
// success, which is why the single call site is behind that check.
func terminalReplacementErr(workloadID, status string) error {
	if !IsFailedReplacementStatus(status) {
		return nil
	}

	return fmt.Errorf(
		"replacement for workload %s ended with status %s; the workload reverted to its previous artifact",
		workloadID, status,
	)
}

// isNotFound reports whether err is the API's 404.
func isNotFound(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}
