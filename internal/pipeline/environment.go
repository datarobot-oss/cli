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

// environment.go contains the typed client wrappers for the pipeline
// execution-environment endpoints described under the
// `pipeline-execution-environments` tag of the pipelines-api OpenAPI spec.
//
// Environments are named, immutable-versioned bags of pip packages that
// pipelines can be built against. They live at the top of the pipelines
// namespace (not nested under a specific pipeline) and have their own
// lifecycle:
//
//   POST   /api/v2/pipelines/environments
//   GET    /api/v2/pipelines/environments
//   PATCH  /api/v2/pipelines/environments/{id}              (adds packages -> new version)
//   DELETE /api/v2/pipelines/environments/{id}              (soft-deletes latest version, cascades parent)
//   DELETE /api/v2/pipelines/environments/{id}/versions/{n} (soft-deletes a specific version)

package pipeline

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/config"
)

// EnvironmentStatus mirrors PipelineEnvironmentStatus in the API.
type EnvironmentStatus string

const (
	EnvironmentStatusCreating EnvironmentStatus = "CREATING"
	EnvironmentStatusReady    EnvironmentStatus = "READY"
	EnvironmentStatusError    EnvironmentStatus = "ERROR"
)

// EnvironmentVersion mirrors PipelineEnvironmentVersionResponse.
type EnvironmentVersion struct {
	Version     int               `json:"version"`
	Packages    []string          `json:"packages"`
	Status      EnvironmentStatus `json:"status"`
	ErrorDetail *string           `json:"errorDetail,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// Environment mirrors PipelineEnvironmentResponse (full detail).
type Environment struct {
	EnvironmentID string               `json:"id"`
	Name          string               `json:"name"`
	Description   *string              `json:"description,omitempty"`
	LatestVersion int                  `json:"latestVersion"`
	Versions      []EnvironmentVersion `json:"versions"`
	CreatedAt     time.Time            `json:"createdAt"`
	UpdatedAt     time.Time            `json:"updatedAt"`
}

// EnvironmentSummary mirrors PipelineEnvironmentSummaryResponse (list item).
type EnvironmentSummary struct {
	EnvironmentID string            `json:"id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	LatestVersion int               `json:"latestVersion"`
	LatestStatus  EnvironmentStatus `json:"latestStatus"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// EnvironmentCreateRequest mirrors PipelineEnvironmentCreateRequest.
type EnvironmentCreateRequest struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Packages    []string `json:"packages"`
}

// EnvironmentUpdateRequest mirrors PipelineEnvironmentUpdateRequest.
type EnvironmentUpdateRequest struct {
	Packages []string `json:"packages"`
}

// CreateEnvironment POSTs a new environment with an initial set of pip
// packages. The API returns 201 with the full Environment payload (a
// single CREATING version is returned immediately; READY status is
// reached asynchronously by the covalent build).
func CreateEnvironment(name, description string, packages []string) (*Environment, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/environments")
	if err != nil {
		return nil, err
	}

	body := EnvironmentCreateRequest{
		Name:     name,
		Packages: packages,
	}
	if description != "" {
		body.Description = &description
	}

	var result Environment

	err = doJSON(http.MethodPost, endpoint, body, "create environment", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListEnvironments returns a paginated slice of environments. The API
// returns a bare JSON array (no envelope), newest first.
func ListEnvironments(offset, limit int) ([]EnvironmentSummary, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/environments")
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

	var page DataPage[EnvironmentSummary]

	err = doJSON(http.MethodGet, endpoint, nil, "environments", &page)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}

// UpdateEnvironment PATCHes an environment with additional packages,
// creating a new immutable version. The response includes the full
// Environment with all versions ordered newest-first.
func UpdateEnvironment(envID string, packages []string) (*Environment, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/environments/" + envID)
	if err != nil {
		return nil, err
	}

	body := EnvironmentUpdateRequest{Packages: packages}

	var result Environment

	err = doJSON(http.MethodPatch, endpoint, body, "update environment", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteEnvironment soft-deletes the most-recent active version of an
// environment. If no active versions remain, the parent environment is
// soft-deleted as well.
func DeleteEnvironment(envID string) error {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/environments/" + envID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete environment")
}

// DeleteEnvironmentVersion soft-deletes a specific version of an
// environment without touching the parent.
func DeleteEnvironmentVersion(envID string, version int) error {
	endpoint, err := config.GetEndpointURL(
		"/api/v2/pipelines/environments/" + envID + "/versions/" + strconv.Itoa(version),
	)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete environment version")
}
