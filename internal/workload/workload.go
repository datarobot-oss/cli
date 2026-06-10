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

	return nil
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

func ListWorkloads(limit int, statuses []string) ([]Workload, error) {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxWorkloadPageSize)))

	for _, s := range statuses {
		query.Add("status", s)
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
