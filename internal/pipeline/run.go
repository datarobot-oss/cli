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

// run.go wraps the pipeline run endpoints described in
// pipelines-api/.../controllers/pipeline_dispatch.py. The CLI exposes the
// same draft/locked URL split as inputs via the shared Scope helpers.
//
// The wire format and server URL paths still use the legacy term
// "dispatch" (e.g. /dispatches, dispatch_id). JSON tags and endpoint
// segments are preserved to keep the API contract intact while the Go
// surface is renamed to "run" to match the new product vocabulary.

package pipeline

import (
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Run lifecycle states (mirrors PipelineDispatchStatus on the wire).
const (
	RunStatusPending   = "PENDING"
	RunStatusRunning   = "RUNNING"
	RunStatusCompleted = "COMPLETED"
	RunStatusFailed    = "FAILED"
	RunStatusCancelled = "CANCELLED"
	RunStatusErrored   = "ERRORED"
)

// Run mirrors PipelineDispatchResponse from the pipelines-api.
type Run struct {
	RunID              string    `json:"id"`
	PipelineID         string    `json:"pipelineId"`
	VersionID          *int      `json:"versionId,omitempty"`
	InputID            string    `json:"inputId"`
	CovalentDispatchID string    `json:"covalentDispatchId,omitempty"`
	TriggeredBy        string    `json:"triggeredBy"`
	Status             string    `json:"status"`
	ErrorDetail        string    `json:"errorDetail,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// RunStatus mirrors PipelineDispatchStatusResponse — the lightweight
// polling-friendly shape returned by GET .../status.
type RunStatus struct {
	RunID              string `json:"id"`
	Status             string `json:"status"`
	CovalentDispatchID string `json:"covalentDispatchId,omitempty"`
}

// RunCreateRequest mirrors PipelineDispatchCreateRequest.
type RunCreateRequest struct {
	InputID string `json:"input_id"`
}

// CreateRun starts a new run for the given input. Returns the
// freshly-created Run (status PENDING).
func CreateRun(pipelineID string, scope Scope, version *int, inputID string) (*Run, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "dispatches")
	if err != nil {
		return nil, err
	}

	body := RunCreateRequest{InputID: inputID}

	var result Run

	err = doJSON(http.MethodPost, endpoint, body, "create run", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListRuns returns a paginated slice of runs for the given scope.
func ListRuns(pipelineID string, scope Scope, version *int, offset, limit int) ([]Run, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "dispatches")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}

	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}

	var page DataPage[Run]

	err = doJSON(http.MethodGet, endpoint, nil, "runs", &page)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}

// GetRun fetches a single run by id within the given scope.
func GetRun(pipelineID string, scope Scope, version *int, runID string) (*Run, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "dispatches/"+runID)
	if err != nil {
		return nil, err
	}

	var run Run

	err = doJSON(http.MethodGet, endpoint, nil, "run", &run)
	if err != nil {
		return nil, err
	}

	return &run, nil
}

// GetRunStatus calls the lightweight GET .../status endpoint useful for
// polling without re-downloading the full run record.
func GetRunStatus(pipelineID string, scope Scope, version *int, runID string) (*RunStatus, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "dispatches/"+runID+"/status")
	if err != nil {
		return nil, err
	}

	var status RunStatus

	err = doJSON(http.MethodGet, endpoint, nil, "run status", &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// CancelRun issues a DELETE on a run, transitioning it to CANCELLED if
// it is still in a non-terminal state.
func CancelRun(pipelineID string, scope Scope, version *int, runID string) error {
	endpoint, err := EndpointFor(pipelineID, scope, version, "dispatches/"+runID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "cancel run")
}
