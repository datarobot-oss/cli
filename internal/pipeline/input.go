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

// input.go contains the typed client wrappers for the pipeline input
// endpoints described in pipelines-api/.../controllers/pipeline_input.py.
// Both draft and locked URL shapes are exercised through Scope/version.

package pipeline

import (
	"net/http"
	"net/url"
	"strconv"
)

// InputState mirrors PipelineInputState in the pipelines-api enums.
type InputState string

const (
	InputStateValid   InputState = "VALID"
	InputStateInvalid InputState = "INVALID"
)

// Input mirrors PipelineInputResponse from the pipelines-api.
type Input struct {
	InputID    string         `json:"input_id"`
	PipelineID string         `json:"pipeline_id"`
	VersionID  *int           `json:"version_id,omitempty"`
	IsDraft    bool           `json:"is_draft"`
	Payload    map[string]any `json:"payload"`
	State      InputState     `json:"state"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

// InputCreateRequest mirrors PipelineInputCreateRequest.
type InputCreateRequest struct {
	Payload map[string]any `json:"payload"`
}

// InputUpdateRequest mirrors PipelineInputUpdateRequest (draft-only).
type InputUpdateRequest struct {
	Payload map[string]any `json:"payload"`
}

// CreateInput POSTs a new input set against the appropriate URL for the
// given scope/version.
func CreateInput(pipelineID string, scope Scope, version *int, payload map[string]any) (*Input, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "inputs")
	if err != nil {
		return nil, err
	}

	body := InputCreateRequest{Payload: payload}

	var result Input

	err = doJSON(http.MethodPost, endpoint, body, "create input", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListInputs returns a paginated slice of inputs for the given scope.
func ListInputs(pipelineID string, scope Scope, version *int, offset, limit int) ([]Input, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "inputs")
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

	// The pipelines-api returns a bare JSON array for list endpoints.
	var inputs []Input

	err = doJSON(http.MethodGet, endpoint, nil, "inputs", &inputs)
	if err != nil {
		return nil, err
	}

	return inputs, nil
}

// GetInput fetches a single input by id within the given scope.
func GetInput(pipelineID string, scope Scope, version *int, inputID string) (*Input, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "inputs/"+inputID)
	if err != nil {
		return nil, err
	}

	var input Input

	err = doJSON(http.MethodGet, endpoint, nil, "input", &input)
	if err != nil {
		return nil, err
	}

	return &input, nil
}

// UpdateInput PATCHes a draft input set with a new payload. Locked inputs
// cannot be updated; the API will return 409 in that case.
func UpdateInput(pipelineID, inputID string, payload map[string]any) (*Input, error) {
	endpoint, err := EndpointFor(pipelineID, ScopeDraft, nil, "inputs/"+inputID)
	if err != nil {
		return nil, err
	}

	body := InputUpdateRequest{Payload: payload}

	var result Input

	err = doJSON(http.MethodPatch, endpoint, body, "update input", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteInput removes an input set within the given scope.
func DeleteInput(pipelineID string, scope Scope, version *int, inputID string) error {
	endpoint, err := EndpointFor(pipelineID, scope, version, "inputs/"+inputID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete input")
}
