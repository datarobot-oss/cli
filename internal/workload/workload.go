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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Workload statuses as serialized by the server (lowercase StrEnum).
const (
	WorkloadStatusUnknown      = "unknown"
	WorkloadStatusSubmitted    = "submitted"
	WorkloadStatusProvisioning = "provisioning"
	WorkloadStatusLaunching    = "launching"
	WorkloadStatusRunning      = "running"
	WorkloadStatusSuspended    = "suspended"
	WorkloadStatusInterrupted  = "interrupted"
	WorkloadStatusStopping     = "stopping"
	WorkloadStatusStopped      = "stopped"
	WorkloadStatusErrored      = "errored"
	WorkloadStatusTerminated   = "terminated"
)

// workloadStatuses indexes every known status for flag validation.
var workloadStatuses = map[string]struct{}{
	WorkloadStatusUnknown:      {},
	WorkloadStatusSubmitted:    {},
	WorkloadStatusProvisioning: {},
	WorkloadStatusLaunching:    {},
	WorkloadStatusRunning:      {},
	WorkloadStatusSuspended:    {},
	WorkloadStatusInterrupted:  {},
	WorkloadStatusStopping:     {},
	WorkloadStatusStopped:      {},
	WorkloadStatusErrored:      {},
	WorkloadStatusTerminated:   {},
}

// ParseWorkloadStatuses lowercases and validates a list of --status values.
func ParseWorkloadStatuses(values []string) ([]string, error) {
	parsed := make([]string, 0, len(values))

	for _, v := range values {
		lower := strings.ToLower(strings.TrimSpace(v))
		if lower == "" {
			continue
		}

		if _, ok := workloadStatuses[lower]; !ok {
			return nil, fmt.Errorf("invalid status %q: use one of %s", v, strings.Join(knownWorkloadStatuses(), ", "))
		}

		parsed = append(parsed, lower)
	}

	return parsed, nil
}

func knownWorkloadStatuses() []string {
	// Stable, lifecycle-ordered listing for error messages and --help.
	return []string{
		WorkloadStatusSubmitted,
		WorkloadStatusProvisioning,
		WorkloadStatusLaunching,
		WorkloadStatusRunning,
		WorkloadStatusSuspended,
		WorkloadStatusInterrupted,
		WorkloadStatusStopping,
		WorkloadStatusStopped,
		WorkloadStatusErrored,
		WorkloadStatusTerminated,
		WorkloadStatusUnknown,
	}
}

// IsTerminalWorkloadStatus reports whether a "wait until running" poll loop
// should stop: running is terminal success; errored and terminated are terminal
// failures. Every other status is non-terminal so the loop keeps polling. That
// deliberately includes the steady non-running states (stopped, suspended,
// interrupted): WaitForWorkload only runs after the caller has created or
// started the workload, so those states are transient on the way up, not a
// reason to bail. A workload that never leaves them surfaces via the timeout
// rather than a spurious "settled" error on the first, still-stale poll.
func IsTerminalWorkloadStatus(s string) bool {
	return IsWorkloadErrorStatus(s) || strings.EqualFold(s, WorkloadStatusRunning)
}

// IsSteadyWorkloadStatus reports whether s is a status the platform will not
// move away from on its own. It is the opposite question to
// IsTerminalWorkloadStatus, which asks whether a workload on its way up has
// arrived: this one asks whether it has stopped moving at all, so the states
// that one deliberately keeps polling through (stopped, suspended,
// interrupted) are answers here rather than steps on the way to one.
//
// Everything left over is the platform moving under its own power: submitted,
// provisioning, launching, stopping, unknown, and whatever it adds later. An
// unrecognised status therefore reads as still moving, which costs a wait and
// never mistakes a transition for a destination.
func IsSteadyWorkloadStatus(s string) bool {
	return IsTerminalWorkloadStatus(s) || IsStoppedWorkloadStatus(s)
}

// IsSuspendedWorkloadStatus reports the one switched-off status a start cannot
// undo. The platform ignores a start while a workload is suspended, so both the
// plan that says what a deploy will do and the apply that refuses to do it have
// to single this status out, and they must not spell it differently.
func IsSuspendedWorkloadStatus(s string) bool {
	return strings.EqualFold(s, WorkloadStatusSuspended)
}

// IsStoppedWorkloadStatus reports whether s is one of the three ways a
// workload can be switched off. They are grouped because a deploy treats them
// alike, and named here because three files ask the same question: this one,
// the deploy's own state reduction, and the status a detached start reports.
//
// Grouped, not equivalent. Only some of them can be started again, which is a
// question for whoever is about to try rather than for this classification.
func IsStoppedWorkloadStatus(s string) bool {
	return strings.EqualFold(s, WorkloadStatusStopped) ||
		strings.EqualFold(s, WorkloadStatusSuspended) ||
		strings.EqualFold(s, WorkloadStatusInterrupted)
}

// IsWorkloadErrorStatus reports whether s is a terminal failure a caller should
// surface as an error rather than a settled state.
//
// Folded, like every other status comparison in this package: the platform
// does not agree with itself about the casing of its enums, and a terminal
// status read as non-terminal is a poll loop that runs to its timeout.
func IsWorkloadErrorStatus(s string) bool {
	return strings.EqualFold(s, WorkloadStatusErrored) || strings.EqualFold(s, WorkloadStatusTerminated)
}

// Workload is the projection of the server's workload document the CLI
// renders. Server-side extras (owners, permissions, creator, stats) are
// deliberately not parsed so they cannot leak into scripted output.
type Workload struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Type       string    `json:"type"`
	Importance string    `json:"importance"`
	ArtifactID string    `json:"artifactId"`
	Endpoint   string    `json:"endpoint"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// WorkloadOutput is the stable JSON shape emitted by --output-format json.
type WorkloadOutput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Type       string `json:"type"`
	Importance string `json:"importance"`
	ArtifactID string `json:"artifactId"`
	Endpoint   string `json:"endpoint"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func NewWorkloadOutput(w Workload) WorkloadOutput {
	return WorkloadOutput{
		ID:         w.ID,
		Name:       w.Name,
		Status:     w.Status,
		Type:       w.Type,
		Importance: w.Importance,
		ArtifactID: w.ArtifactID,
		Endpoint:   w.Endpoint,
		CreatedAt:  w.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  w.UpdatedAt.Format(time.RFC3339),
	}
}

// WorkloadStatusOutput is the stable JSON shape emitted by
// `dr workload status --output-format json`; the full document stays the
// domain of `dr workload get`.
type WorkloadStatusOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type workloadCreateRequest struct {
	Name       string          `json:"name"`
	ArtifactID string          `json:"artifactId"`
	Artifact   json.RawMessage `json:"artifact"`
}

// ValidateWorkloadCreateRequest checks the structural invariants of a
// user-supplied workload spec (required name, exactly one of artifactId or
// inline artifact) and lets the server validate field-level shape. Unknown
// fields are not rejected for the same reason as ValidateCreateRequest: the
// server's 422 carries a JSON-path detail that's clearer than what
// DisallowUnknownFields would produce. The original bytes are sent verbatim.
func ValidateWorkloadCreateRequest(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))

	var req workloadCreateRequest

	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	if req.Name == "" {
		return errors.New("invalid spec: required field 'name' is missing or empty")
	}

	hasArtifactID := req.ArtifactID != ""
	hasArtifact := len(req.Artifact) > 0 && string(req.Artifact) != "null"

	if hasArtifactID == hasArtifact {
		return errors.New("invalid spec: exactly one of 'artifactId' (existing artifact) or 'artifact' (inline definition) must be set")
	}

	return validateRuntimeReplicaAutoscaling(data)
}

// validateRuntimeReplicaAutoscaling rejects replicaCount alongside autoscaling.enabled=true
// per container group, matching workload-api GroupRuntime reconciliation. Other runtime
// shape checks stay on the server (422 with JSON-path detail).
func validateRuntimeReplicaAutoscaling(data []byte) error {
	var doc map[string]any

	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}

	runtime, ok := doc["runtime"].(map[string]any)
	if !ok {
		return nil
	}

	groups, ok := runtime["containerGroups"].([]any)
	if !ok {
		return nil
	}

	for i, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}

		if err := validateContainerGroupAutoscaling(i, group); err != nil {
			return err
		}
	}

	return nil
}

func validateContainerGroupAutoscaling(index int, group map[string]any) error {
	autoscalingRaw, ok := group["autoscaling"]
	if !ok {
		return nil
	}

	if _, ok := autoscalingRaw.(bool); ok {
		return fmt.Errorf(
			"invalid spec: runtime.containerGroups[%d]: autoscaling must be an object, not a boolean",
			index,
		)
	}

	autoscaling, ok := autoscalingRaw.(map[string]any)
	if !ok {
		return nil
	}

	if enabled, ok := autoscaling["enabled"]; ok && enabled != nil {
		if _, ok := enabled.(bool); !ok {
			return fmt.Errorf(
				"invalid spec: runtime.containerGroups[%d]: autoscaling.enabled must be true or false",
				index,
			)
		}
	}

	if replicaCountSet(group) && autoscalingEnabled(group) {
		return fmt.Errorf(
			"invalid spec: runtime.containerGroups[%d]: replicaCount and autoscaling.enabled=true are mutually exclusive; omit replicaCount when using autoscaling or set autoscaling.enabled to false",
			index,
		)
	}

	return nil
}

func replicaCountSet(group map[string]any) bool {
	v, ok := group["replicaCount"]
	if !ok {
		return false
	}

	return v != nil
}

func autoscalingEnabled(group map[string]any) bool {
	autoscaling, ok := group["autoscaling"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := autoscaling["enabled"]
	if !ok || enabled == nil {
		return true
	}

	b, ok := enabled.(bool)

	return ok && b
}

// CreateWorkload POSTs payload to /api/v2/workloads/ and returns the parsed
// workload. payload is typically a json.RawMessage from the spec file, sent
// verbatim after ValidateWorkloadCreateRequest passed. The server replies
// 201 with the full workload document inline; endpoint is present from
// creation (it is a stable gateway URL, not a liveness signal).
func CreateWorkload(payload any) (*Workload, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/")
	if err != nil {
		return nil, err
	}

	var workload Workload

	err = drapi.PostJSON(url, "workload", payload, &workload)
	if err != nil {
		return nil, err
	}

	return &workload, nil
}

// escapeID percent-encodes a user-supplied resource id so it always stays a
// single URL path segment.
func escapeID(id string) string {
	return url.PathEscape(id)
}

func GetWorkload(workloadID string) (*Workload, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/")
	if err != nil {
		return nil, err
	}

	var workload Workload

	err = drapi.GetJSON(url, "workload", &workload)
	if err != nil {
		return nil, err
	}

	return &workload, nil
}

// Serving is what a wait must see before it settles. The zero value asks only
// that the workload be running, which is what a create and a start want: both
// leave the running artifact where it was and neither replaces a generation.
//
// It is a struct rather than two arguments because the difference between the
// three cases is not expressible in one string. A resize confirms no artifact
// and still has to wait out a drain; a create confirms no artifact and must
// not. Spelling that as an empty id made the two indistinguishable at the call
// site and put a drain wait in front of every create.
type Serving struct {
	// ArtifactID is the version that must be the one answering, empty when
	// the run changed none.
	ArtifactID string

	// AwaitDrain requires the generation being replaced to have stopped
	// taking requests. A roll and a resize both replace one; a create and a
	// start do not.
	AwaitDrain bool

	// OnUnconfirmed, if set, is called once with a short reason when the
	// proton list cannot be read and the wait falls back to the artifact
	// alone. A caller that reports on the endpoint afterwards should say so,
	// because on that path the endpoint may still be answering from the
	// version being replaced.
	OnUnconfirmed func(reason string)
}

// WaitForWorkload polls GetWorkload on interval until the workload reaches a
// terminal status (see IsTerminalWorkloadStatus) and satisfies want, or the
// deadline expires. A timeout returns the last-seen workload so a caller can
// say where it got to; errored and terminated come back as failures.
//
// A roll names its candidate, and needs to, because the platform rolls by
// replacing the workload with itself and so reports "running" from before the
// swap until after it, leaving a status-only wait to return on the first poll
// having proved nothing. The artifact is not sufficient on its own either,
// which is what protonsServing covers.
//
// The proton list is corroboration, never the rollout itself, so no failure to
// read it fails a deploy. A refusal falls back at once; anything else is given
// a few polls to recover and then falls back too, because a deploy that has
// built, promoted and started serving must not be reported as failed on
// account of the second request that was only checking.
//
// onTick may be nil and is invoked after each poll, mirroring WaitForBuild.
func WaitForWorkload(
	workloadID string,
	want Serving,
	interval, timeout time.Duration,
	onTick func(*Workload),
) (*Workload, error) {
	var unreadable, empty int

	// The error term is first and unguarded: an errored workload still reports
	// whatever it was running, so requiring the artifact too would poll a dead
	// workload to its timeout instead of failing on first sight.
	stop := func(wl *Workload) bool {
		if IsWorkloadErrorStatus(wl.Status) {
			return true
		}

		// Only running reaches here, the error statuses having returned above.
		if !IsTerminalWorkloadStatus(wl.Status) || !onWantedArtifact(wl, want.ArtifactID) {
			return false
		}

		verdict, err := servingWanted(workloadID, want)
		if err != nil {
			unreadable++
			if unreadable <= maxTransientPollErrors {
				return false
			}

			degrade(want, "the container generations could not be read: "+err.Error())

			return true
		}

		unreadable = 0

		if !verdict.Empty {
			empty = 0

			return verdict.Serving
		}

		// An empty list that keeps coming back is a route with nothing to say
		// rather than a handover in progress. Bounded the same way an
		// unwatched replacement 404 is, and for the same reason: waiting
		// forever on an absence turns a finished deploy into a timeout.
		empty++
		if empty < uncorroboratedAbsences {
			return false
		}

		degrade(want, "this workload lists no container generations")

		return true
	}

	wl, err := pollWorkload(workloadID, interval, timeout, stop, onTick)
	if err != nil {
		return wl, unpromoted(workloadID, wl, want, err)
	}

	if IsWorkloadErrorStatus(wl.Status) {
		return wl, fmt.Errorf("workload %s ended with status %s; run 'dr workload logs %s' to inspect",
			workloadID, wl.Status, workloadID)
	}

	return wl, nil
}

// degrade tells the caller the handover went unconfirmed, once.
func degrade(want Serving, reason string) {
	if want.OnUnconfirmed != nil {
		want.OnUnconfirmed(reason)
	}
}

// onWantedArtifact reports whether wl names the artifact the caller named; an
// empty want is a caller that changed none.
func onWantedArtifact(wl *Workload, wantArtifactID string) bool {
	return wantArtifactID == "" || wl.ArtifactID == wantArtifactID
}

// servingWanted asks the proton list whether the previous generation has
// stopped answering.
//
// A refusal is not a failure of the rollout. The route is absent on some
// installs (404), a token that may deploy is not guaranteed to be allowed to
// list protons (403), and a gateway that does not know the path can answer 405
// or 422. None of those say anything went wrong, so they fall back to the
// artifact check immediately. Everything else, including a body this code
// cannot read, is returned for the caller to retry and then forgive.
func servingWanted(workloadID string, want Serving) (protonVerdict, error) {
	// A create and a start replace no generation and confirm no artifact, so
	// there is nothing here for them to ask about and no reason to spend a
	// request finding that out.
	if want.ArtifactID == "" && !want.AwaitDrain {
		return protonVerdict{Serving: true}, nil
	}

	protons, err := ListProtons(workloadID)
	if err != nil {
		if refusedRoute(err) {
			degrade(want, "this install did not answer for container generations ("+err.Error()+")")

			return protonVerdict{Serving: true}, nil
		}

		return protonVerdict{}, err
	}

	return protonsServing(protons, want), nil
}

// refusedRoute reports whether err is the route saying no in a way that will
// not change on the next poll.
func refusedRoute(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500
}

// unpromoted says which way a rollout stalled, since a bare timeout sends the
// reader looking for something that is not stuck.
//
// Only a deadline is decorated. A wait whose polls failed already carries the
// failure, and inferring anything about a rollout from a workload read taken
// seconds into it would be a guess dressed as a diagnosis.
func unpromoted(workloadID string, wl *Workload, want Serving, err error) error {
	var timedOut *waitTimeoutError

	if wl == nil || !errors.As(err, &timedOut) {
		return err
	}

	if !onWantedArtifact(wl, want.ArtifactID) {
		return fmt.Errorf(
			"workload %s is %s but still serving artifact %s, not %s, so the rollout has not promoted "+
				"the new version; check 'dr workload status %s': %w",
			workloadID, wl.Status, wl.ArtifactID, want.ArtifactID, workloadID, err)
	}

	if reason := stalledOn(workloadID, want); reason != "" {
		return fmt.Errorf("workload %s reports artifact %s, but %s; check 'dr workload status %s': %w",
			workloadID, wl.ArtifactID, reason, workloadID, err)
	}

	return err
}

// stalledOn names what the proton list was still showing when the wait ran out,
// or "" when it cannot say. The read is on the error path only and its own
// failure is swallowed: this decorates a timeout already decided.
func stalledOn(workloadID string, want Serving) string {
	protons, err := ListProtons(workloadID)
	if err != nil {
		return ""
	}

	if len(protons) == 0 {
		return "no container generations are listed for it"
	}

	if want.AwaitDrain && anyServingPredecessor(protons, want.ArtifactID) {
		return "a previous version is still answering its endpoint, so the rollout has not finished"
	}

	if want.ArtifactID != "" && !anyRunning(protons, want.ArtifactID) {
		return "nothing is running that version yet"
	}

	return ""
}

// WaitForSteadyWorkload polls until the workload stops moving under its own
// power (see IsSteadyWorkloadStatus) or deadline expires. It is for a caller
// that has to know where a transition landed before it can decide anything,
// rather than one waiting for a workload it just started to come up.
//
// The difference from WaitForWorkload is the whole reason both exist. That one
// polls through stopped, suspended and interrupted, because for a workload on
// its way up they are steps rather than destinations; this one returns at them.
// A `stopping` workload handed to WaitForWorkload runs to its timeout.
//
// Errored and terminated come back with a nil error. They are legitimate
// answers to "has it stopped moving", so the verdict belongs to the caller
// rather than here: the same "errored" is a destination to a caller asking
// where a transition landed and a failure to one waiting for a workload it
// just started. A caller that treats a nil error as "healthy" is reading this
// as the wrong function; that is what WaitForWorkload is for.
func WaitForSteadyWorkload(
	workloadID string,
	interval, timeout time.Duration,
	onTick func(*Workload),
) (*Workload, error) {
	return pollWorkload(workloadID, interval, timeout, func(wl *Workload) bool {
		return IsSteadyWorkloadStatus(wl.Status)
	}, onTick)
}

// waitTimeoutError marks a wait that ran out of time, so a caller can tell it
// from one whose polls failed and decorate only the first.
type waitTimeoutError struct {
	workloadID string
	timeout    time.Duration
}

func (e *waitTimeoutError) Error() string {
	return fmt.Sprintf("timeout waiting for workload %s after %s", e.workloadID, e.timeout)
}

// pollWorkload is the loop both waits share. It errors only on a failed poll
// and on the deadline, leaving the verdict on where it stopped to the caller.
//
// stop takes the whole workload because "has it arrived" is not always about
// the status: a roll leaves the status reading "running" throughout. It cannot
// fail: confirming a rollout takes a second request, but a failure there is the
// corroboration falling over rather than the deploy, so stop absorbs it and
// this loop only ever reports on the workload read it names.
//
// The last-seen workload comes back with every error that has one, so a caller
// can still say what the workload was doing when the wait gave up.
func pollWorkload(
	workloadID string,
	interval, timeout time.Duration,
	stop func(*Workload) bool,
	onTick func(*Workload),
) (*Workload, error) {
	if interval <= 0 {
		interval = defaultWorkloadPollInterval
	}

	deadline := time.Now().Add(timeout)
	budget := transientBudget{}

	var last *Workload

	for {
		wl, err := GetWorkload(workloadID)
		if err != nil {
			if !budget.forgive(err) {
				return last, fmt.Errorf("poll workload %s: %w", workloadID, err)
			}
		} else {
			last = wl

			if onTick != nil {
				onTick(wl)
			}

			if stop(wl) {
				return wl, nil
			}

			budget.reset()
		}

		if time.Now().After(deadline) {
			return last, &waitTimeoutError{workloadID: workloadID, timeout: timeout}
		}

		time.Sleep(interval)
	}
}

// defaultWorkloadPollInterval keeps an exported poller from busy-spinning when
// handed a non-positive interval, matching defaultReplacementPollInterval. The
// CLI's own flags cannot produce one, but nothing in the signature says so, and
// this loop now runs for as long as a rollout takes rather than a few polls.
const defaultWorkloadPollInterval = 2 * time.Second

type WorkloadList struct {
	Data       []Workload `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

// maxPageSize is the server-enforced ceiling on the list endpoints' limit query
// param (1..100); larger values are rejected with a 422, so the page size is
// clamped and larger totals are satisfied via next-links. Shared by the
// workload and artifact list endpoints, which enforce the same ceiling.
const maxPageSize = 100

// ListWorkloads fetches up to limit workloads starting at offset, optionally
// filtered by status and by the name of the Enclave they actually run on.
func ListWorkloads(limit, offset int, statuses []string, enclave string) ([]Workload, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	if offset < 0 {
		return nil, fmt.Errorf("invalid offset %d: must be non-negative", offset)
	}

	// Trim so a copied name with stray spaces matches, same as the pin side.
	enclave = strings.TrimSpace(enclave)

	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxPageSize)))
	query.Set("offset", strconv.Itoa(offset))

	for _, s := range statuses {
		query.Add("status", s)
	}

	if enclave != "" {
		query.Set("enclave", enclave)
	}

	pageURL, err := drapi.EndpointURL("/workloads/", query)
	if err != nil {
		return nil, err
	}

	return paginateWorkloads(pageURL, limit)
}

// paginateWorkloads follows next-links from pageURL, accumulating workloads
// until limit is reached or the pages run out. It is split from ListWorkloads
// to keep that function's cyclomatic complexity under the linter threshold.
func paginateWorkloads(pageURL string, limit int) ([]Workload, error) {
	var all []Workload

	for pageURL != "" {
		var list WorkloadList

		if err := drapi.GetJSON(pageURL, "workloads", &list); err != nil {
			return nil, err
		}

		all = append(all, list.Data...)

		if len(all) >= limit {
			return all[:limit], nil
		}

		if list.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return nil, err
		}

		pageURL = list.Next
	}

	return all, nil
}

// DeleteWorkload deletes a workload. The server stops the backing proton(s)
// first, so deleting a running workload is legal; on a proton-stop failure
// it replies 502 with a "please retry" detail.
func DeleteWorkload(workloadID string) error {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/")
	if err != nil {
		return err
	}

	return drapi.DeleteJSON(url, "workload", nil, nil)
}
