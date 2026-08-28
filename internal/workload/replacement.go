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
	"errors"
	"fmt"
	"net/http"
	"strings"
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
// That stance is not free: a failure status the platform adds later would be
// classified as still in progress, and if the record is then cleared,
// WaitForReplacement reports the rollout as a success. That "errored" had to be
// added at all is proof the vocabulary was incomplete once already.
//
// A roll no longer rests on this: the deploy confirms afterwards which artifact
// the workload ended up running (see WaitForWorkload). A resize has no artifact
// change to confirm, and is covered instead by that same wait refusing to
// settle while a generation is still draining.
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
//
// The comparison is case-insensitive. To be plain about why: this route has
// only ever been seen answering in lower case, so the fold is hardening rather
// than a fix for something observed. What justifies it is that the platform is
// not consistent across resources, its build statuses being upper case where
// these and the workload's are lower, and that the artifact's lock check
// already folds for the same reason. The cost of being wrong is one-sided:
// reading a settled "COMPLETED" as still in progress would refuse every deploy
// for as long as the record lingers, with a message calling a finished
// replacement one in progress.
func IsTerminalReplacementStatus(s string) bool {
	return IsFailedReplacementStatus(s) || strings.EqualFold(s, ReplacementStatusCompleted)
}

// IsFailedReplacementStatus reports whether s is a terminal failure. On
// failure the workload reverts to the artifact it was running before the
// replacement started, so a failed rollout never promotes.
func IsFailedReplacementStatus(s string) bool {
	return strings.EqualFold(s, ReplacementStatusFailed) || strings.EqualFold(s, ReplacementStatusErrored)
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
// RefuseActiveReplacement, which asks the same question with the ambiguity
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
// RefuseActiveReplacement first.
//
// Both artifacts must share draft or locked status; the platform rejects a
// mismatch, which is why a locked workload has to have its successor locked
// before this is called. That is not enforced here because the caller is the
// one holding both statuses, and the lock has to happen between the guard and
// this call.
//
// runtime is optional and may be nil. When a deploy changes the sizing as well
// as the version, sending both here is what brings the candidate up under the
// sizing it was asked for: applying them as two operations either resizes the
// version being rolled off, or starts the new one under the old sizing, which
// is how a larger model meets a bundle that cannot hold it.
func StartReplacement(workloadID, artifactID string, runtime json.RawMessage) (*Replacement, error) {
	url, err := replacementURL(workloadID)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"artifactId": artifactID,
		"strategy":   rollingStrategy,
	}

	// Omitted rather than sent as null when there is nothing to change, so a
	// rollout that only swaps the version does not read as a settings change
	// as well.
	if len(runtime) > 0 {
		body["runtime"] = runtime
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

// RefuseActiveReplacement refuses to proceed when workloadID already has a
// replacement in flight, because starting another queues a second swap instead
// of erroring.
//
// How many times to call it follows from what sits between the check and the
// call it protects. A roll mints and may lock an artifact in between, so it
// runs this twice: once up front, so a doomed command fails before it makes
// anything, and once immediately before the swap, which is the check that
// actually holds, since the live state can change in between. A caller with
// nothing in between runs it once, because there the two are the same moment.
//
// A refusal wraps ErrReplacementInFlight; everything else it returns is a
// failure to find out. Callers that want to say which operation was stopped
// should wrap the second kind and leave the first alone, since the refusal
// already names the workload and the remedy.
//
// It reads an empty answer as "nothing in flight", which is only safe because
// every caller has already established the workload exists. The replacement
// route answers the same 404 for "nothing in flight" and for "no such
// workload", so a caller that has not settled that first would wave through
// exactly the mistyped id it means to catch. `up` settles it in Look, before
// the plan is even built.
func RefuseActiveReplacement(workloadID string) error {
	active, err := GetActiveReplacement(workloadID)
	if err != nil {
		return err
	}

	if active == nil {
		return nil
	}

	return refuseActive(workloadID, active)
}

// refuseActive is the verdict both entry points share once a record is in hand.
func refuseActive(workloadID string, active *Replacement) error {
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
		"workload %s: %w%s; wait for it to settle before starting another",
		workloadID, ErrReplacementInFlight, describeActive(active),
	)
}

// ErrReplacementInFlight marks the refusal, so a caller can tell a rollout it
// should wait out from a failure to find out whether there is one. The two
// want different words: the first is a state, the second is a read that did
// not happen, and a caller that wrapped both would put "cannot tell whether"
// in front of a sentence that already knows.
//
// It reads as a whole sentence rather than as a fragment of the message above,
// because a sentinel is the one error a caller may hand on by itself.
var ErrReplacementInFlight = errors.New("a replacement is already in progress")

// describeActive names what is in flight, carrying only the fields the
// platform actually filled in.
//
// The artifact is one of them because a settings change rolls the workload
// onto the artifact it is already running, and the platform returns no
// candidate for it. Naming one unconditionally printed an empty field
// mid-sentence, or the id of the version the plan had just reported as live,
// which reads as a rollout onto itself.
func describeActive(active *Replacement) string {
	parts := make([]string, 0, 2)

	if active.ArtifactID != "" {
		parts = append(parts, "to artifact "+active.ArtifactID)
	}

	if active.Status != "" {
		parts = append(parts, "status "+active.Status)
	}

	if len(parts) == 0 {
		return ""
	}

	return " (" + strings.Join(parts, ", ") + ")"
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
//   - the last-seen replacement and an error, when polling failed; that is the
//     seed when nothing was ever observed, so its status is what StartReplacement
//     answered rather than anything this wait saw
//   - nil and an error, when nothing was ever active
//
// The two 404 cases mean opposite things, which is why this is not a plain
// poll loop. Seen after a status, a 404 means the platform settled the
// rollout and collected the record, which is success. Seen before any status,
// it means either that nothing was ever active or that the platform settled a
// fast rollout before the first poll landed; passing started is what lets the
// wait tell those from each other.
//
// A 404 before any record has been watched has to earn its meaning: the first
// poll fires immediately after the POST, and a route that has not caught up
// answers exactly like one whose rollout is over, so it is counted rather than
// acted on.
//
// A settled rollout does not guarantee a terminal Status on the way out. When
// the record is collected before the first poll lands, the seed is the only
// record there is, so what comes back is started itself, carrying whatever
// the POST answered, typically "submitted". A caller that renders a final
// status should ask IsTerminalReplacementStatus rather than read a nil error
// as "completed".
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
	wait := &replacementWait{workloadID: workloadID, lastSeen: started, onTick: onTick}

	for {
		done, err := wait.step()
		if done {
			return wait.lastSeen, err
		}

		if time.Now().After(deadline) {
			return wait.lastSeen, timedOut(workloadID, timeout)
		}

		time.Sleep(interval)
	}
}

// replacementWait is one wait's state, held together so the loop above stays a
// loop and the decisions live in step.
type replacementWait struct {
	workloadID string
	lastSeen   *Replacement
	onTick     func(*Replacement)
	watched    bool
	absences   int
	budget     transientBudget
}

// step polls once and reports whether the wait is over, with the error to
// return when it is.
func (w *replacementWait) step() (bool, error) {
	replacement, err := GetActiveReplacement(w.workloadID)
	if err != nil {
		// A roll now spends minutes on this route, which is long enough for
		// one 502 or one dropped connection to land between healthy polls, and
		// failing there leaves the swap in flight with the deploy reporting
		// failure.
		if w.budget.forgive(err) {
			return false, nil
		}

		return true, fmt.Errorf("poll replacement for workload %s: %w", w.workloadID, err)
	}

	w.budget.reset()

	if replacement == nil {
		w.absences++

		settled, done, absenceErr := absenceMeans(w.workloadID, w.lastSeen, w.watched, w.absences)
		w.lastSeen = settled

		return done, absenceErr
	}

	w.watched = true
	w.absences = 0
	w.lastSeen = replacement

	if w.onTick != nil {
		w.onTick(replacement)
	}

	if !IsTerminalReplacementStatus(replacement.Status) {
		return false, nil
	}

	return true, terminalReplacementErr(w.workloadID, replacement.Status)
}

// timedOut is the deadline error, spelled once because two paths reach it.
func timedOut(workloadID string, timeout time.Duration) error {
	return fmt.Errorf("timeout waiting for replacement on workload %s after %s", workloadID, timeout)
}

// absenceMeans reads a 404 off the replacement route, which says three things
// depending on what the wait has seen: nothing seen and nothing seeded is the
// attach path, gone after being watched is a finished rollout, and gone without
// ever being watched settles only after uncorroboratedAbsences. done reports
// whether the wait is over.
func absenceMeans(workloadID string, lastSeen *Replacement, watched bool, absences int) (*Replacement, bool, error) {
	switch {
	case lastSeen == nil:
		return nil, true, fmt.Errorf("no replacement is in flight for workload %s", workloadID)

	case watched, absences >= uncorroboratedAbsences:
		return lastSeen, true, nil

	default:
		return lastSeen, false, nil
	}
}

// uncorroboratedAbsences is how many consecutive 404s it takes to believe a
// rollout the wait never watched is over. One is not enough: the first lands
// immediately after the POST, before the record is necessarily readable.
const uncorroboratedAbsences = 3

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
