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

// WaitForWorkload polls GetWorkload on interval until the workload reaches a
// terminal status (see IsTerminalWorkloadStatus) while serving wantArtifactID,
// or the deadline expires. It returns the final Workload in every
// non-poll-error case so callers can render it:
//   - running the wanted artifact: success, nil error.
//   - errored/terminated: the final workload alongside a failure error.
//   - timeout: the last-seen workload alongside a timeout error (this covers a
//     workload stuck in a non-running steady state such as stopped, and one
//     that is running but has never promoted the version the caller put there).
//
// wantArtifactID is the artifact the caller expects to find serving, and empty
// when it has no expectation. A create, a start and a resize all leave the
// running artifact where it was, so there is nothing for them to name; a roll
// is the case this exists for.
//
// Naming it is what makes the wait mean anything there. The platform applies a
// roll by replacing the workload with itself, and the version already serving
// keeps serving for the whole of the swap, so the workload reports "running"
// from before the rollout starts until after it ends. Waiting on the status
// alone therefore returns on the first poll, on the old version, having proved
// nothing: the command exits reporting success while the endpoint still answers
// with the previous build.
//
// Ids are compared exactly, unlike every status comparison in this package.
// Those fold because the platform does not agree with itself about the casing
// of its enums. An object id is not an enum, and folding one would only hide a
// mismatch worth seeing.
//
// onTick may be nil and is invoked after each poll for debug-only observation,
// mirroring WaitForBuild.
func WaitForWorkload(
	workloadID, wantArtifactID string,
	interval, timeout time.Duration,
	onTick func(*Workload),
) (*Workload, error) {
	// The error term comes first, and is deliberately not guarded by the
	// artifact check. An errored workload is still reporting whatever it was
	// running, which is never the artifact a roll is waiting for, so requiring
	// both would poll a dead workload to its timeout instead of failing on the
	// first sight of it.
	stop := func(wl *Workload) bool {
		return IsWorkloadErrorStatus(wl.Status) ||
			(IsTerminalWorkloadStatus(wl.Status) && serving(wl, wantArtifactID))
	}

	wl, err := pollWorkload(workloadID, interval, timeout, stop, onTick)
	if err != nil {
		return wl, unpromoted(workloadID, wl, wantArtifactID, err)
	}

	if wl == nil {
		return wl, nil
	}

	if IsWorkloadErrorStatus(wl.Status) {
		return wl, fmt.Errorf("workload %s ended with status %s; run 'dr workload logs %s' to inspect",
			workloadID, wl.Status, workloadID)
	}

	return wl, nil
}

// serving reports whether wl is running the artifact the caller named.
//
// An empty want is not a hole in the check: it is a caller that changed no
// artifact, for whom the status is the whole question.
func serving(wl *Workload, wantArtifactID string) bool {
	return wantArtifactID == "" || wl.ArtifactID == wantArtifactID
}

// unpromoted replaces a bare timeout with what actually went wrong when the
// workload was there all along and simply never took the new version.
//
// "timeout waiting for workload X" sends the reader looking for something that
// is not stuck. Naming both artifacts says the rollout is what did not land and
// that the old version is still the one answering, which is the difference
// between reading logs and re-running the deploy.
func unpromoted(workloadID string, wl *Workload, wantArtifactID string, err error) error {
	if wl == nil || serving(wl, wantArtifactID) {
		return err
	}

	return fmt.Errorf(
		"workload %s is %s but still serving artifact %s, not %s, so the rollout has not promoted "+
			"the new version; check 'dr workload status %s': %w",
		workloadID, wl.Status, wl.ArtifactID, wantArtifactID, workloadID, err)
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

// pollWorkload is the loop both waits share. It errors only on a poll that
// failed and on the deadline, so the verdict on a workload the loop stopped at
// is the caller's to pass: the same "errored" is a failure to one of them and an
// answer to the other.
//
// stop is handed the whole workload rather than its status because "has it
// arrived" is not always a question about the status. A roll leaves the status
// reading "running" from beginning to end, and the only thing that moves is
// which artifact the workload names.
//
// The last-seen workload comes back with a timeout so a caller can still say
// where it got to, and nothing at all comes back from a failed poll, since a
// wait that could not read the workload has no state to report.
//
// A transient failure is not a failed poll. These waits now run for as long as
// a rollout takes, which is long enough for one 502 or one dropped connection
// to land between two healthy polls, and failing a deploy that is going fine is
// a worse answer than looking again. The allowance resets on every good poll,
// so it forgives blips without ever forgiving an endpoint that has gone away.
func pollWorkload(
	workloadID string,
	interval, timeout time.Duration,
	stop func(*Workload) bool,
	onTick func(*Workload),
) (*Workload, error) {
	deadline := time.Now().Add(timeout)

	var (
		last      *Workload
		transient int
	)

	for {
		wl, err := GetWorkload(workloadID)

		switch {
		case err == nil:
			transient = 0
			last = wl

			if onTick != nil {
				onTick(wl)
			}

			if stop(wl) {
				return wl, nil
			}

		case isTransientPollError(err) && transient < maxTransientPollErrors:
			transient++

		default:
			return nil, fmt.Errorf("poll workload %s: %w", workloadID, err)
		}

		if time.Now().After(deadline) {
			return last, fmt.Errorf("timeout waiting for workload %s after %s", workloadID, timeout)
		}

		time.Sleep(interval)
	}
}

type WorkloadList struct {
	Data       []Workload `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

// maxWorkloadPageSize is the server-enforced ceiling on the list endpoint's
// limit query param (1..100); larger values are rejected with a 422, so the
// page size is clamped and larger totals are satisfied via next-links.
const maxWorkloadPageSize = 100

// ListWorkloads fetches up to limit workloads, optionally filtered by status
// and by the name of the Enclave they actually run on.
func ListWorkloads(limit int, statuses []string, enclave string) ([]Workload, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	// Trim so a copied name with stray spaces matches, same as the pin side.
	enclave = strings.TrimSpace(enclave)

	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxWorkloadPageSize)))

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
