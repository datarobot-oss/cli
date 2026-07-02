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

// image.go contains the typed client wrappers for the pipeline
// execution-image endpoints described under the
// `pipeline-execution-images` tag of the pipelines-api OpenAPI spec.
//
// Images are named, immutable-versioned execution environments (pip/conda
// packages, base image, NVIDIA GPU support). They live at the top of the pipelines namespace (not
// nested under a specific pipeline) and have their own lifecycle:
//
//	POST   /api/v2/pipelines/images
//	GET    /api/v2/pipelines/images
//	GET    /api/v2/pipelines/images/{id}
//	PATCH  /api/v2/pipelines/images/{id}              (replacement -> new version)
//	DELETE /api/v2/pipelines/images/{id}              (soft-deletes latest version, cascades parent)
//	DELETE /api/v2/pipelines/images/{id}/versions/{n} (soft-deletes a specific version)
//	GET    /api/v2/pipelines/images/{id}/versions/{n}/logs
package pipeline

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/config"
)

// ImageStatus mirrors PipelineImageStatus in the API.
type ImageStatus string

// CondaSpec mirrors CondaSpec from the pipelines-api: a structured conda
// definition with explicit channels and dependencies lists.
type CondaSpec struct {
	Channels     []string `json:"channels"`
	Dependencies []string `json:"dependencies"`
}

// CondaValue holds a conda definition that serialises as either a plain
// package list (when Channels is empty) or a CondaSpec object (when
// Channels is provided). Matches the anyOf[[]string, CondaSpec] schema.
type CondaValue struct {
	Deps     []string
	Channels []string
}

// MarshalJSON renders a plain []string when no channels are set, or a
// CondaSpec object when channels are provided.
func (c CondaValue) MarshalJSON() ([]byte, error) {
	if len(c.Channels) > 0 {
		return json.Marshal(CondaSpec{
			Channels:     c.Channels,
			Dependencies: c.Deps,
		})
	}

	return json.Marshal(c.Deps)
}

// UnmarshalJSON accepts both the string-array form and the CondaSpec
// object form returned by the pipelines-api.
func (c *CondaValue) UnmarshalJSON(data []byte) error {
	var deps []string

	if err := json.Unmarshal(data, &deps); err == nil {
		c.Channels = nil
		c.Deps = deps

		return nil
	}

	var spec CondaSpec

	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}

	c.Channels = spec.Channels
	c.Deps = spec.Dependencies

	return nil
}

const (
	ImageStatusCreating ImageStatus = "CREATING"
	ImageStatusReady    ImageStatus = "READY"
	ImageStatusError    ImageStatus = "ERROR"
)

// ImageDefinition mirrors PipelineImageDefinition — the canonical
// definition stored on a PipelineImageVersion row and round-tripped
// verbatim on every read.
//
// JSON tags match the response wire format: "packages" and "pythonBaseImage".
// Request structs (ImageCreateRequest, ImageUpdateRequest) use the "pip" /
// "baseImage" aliases accepted by the API's backward-compat validator.
type ImageDefinition struct {
	Name      string      `json:"name"`
	Pip       []string    `json:"packages"`
	Conda     *CondaValue `json:"conda,omitempty"`
	BaseImage *string     `json:"pythonBaseImage,omitempty"`
	Nvidia    bool        `json:"nvidia"`
}

// ImageVersion mirrors PipelineImageVersionResponse.
type ImageVersion struct {
	Version     int             `json:"version"`
	Definition  ImageDefinition `json:"definition"`
	Status      ImageStatus     `json:"status"`
	ErrorDetail *string         `json:"errorDetail,omitempty"`
	ImageURI    *string         `json:"imageUri,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Image mirrors PipelineImageResponse (full detail).
type Image struct {
	ImageID       string         `json:"id"`
	Name          string         `json:"name"`
	Description   *string        `json:"description,omitempty"`
	LatestVersion int            `json:"latestVersion"`
	Versions      []ImageVersion `json:"versions"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// ImageSummary mirrors PipelineImageSummaryResponse (list item).
type ImageSummary struct {
	ImageID       string      `json:"id"`
	Name          string      `json:"name"`
	Description   *string     `json:"description,omitempty"`
	LatestVersion int         `json:"latestVersion"`
	LatestStatus  ImageStatus `json:"latestStatus"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
}

// ImageCreateRequest mirrors PipelineImageCreateRequest.
type ImageCreateRequest struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Pip         []string    `json:"pip,omitempty"`
	Conda       *CondaValue `json:"conda,omitempty"`
	BaseImage   *string     `json:"baseImage,omitempty"`
	Nvidia      bool        `json:"nvidia,omitempty"`
}

// ImageUpdateRequest mirrors PipelineImageUpdateRequest.
// Name is required by the API; the server overrides it with the parent
// image's canonical name so all versions share the same name.
type ImageUpdateRequest struct {
	Name      string      `json:"name"`
	Pip       []string    `json:"pip,omitempty"`
	Conda     *CondaValue `json:"conda,omitempty"`
	BaseImage *string     `json:"baseImage,omitempty"`
	Nvidia    bool        `json:"nvidia,omitempty"`
}

// CreateImage POSTs a new image. The API returns 201 with the full Image
// payload (a single CREATING version is returned immediately; READY status
// is reached asynchronously by the covalent build).
func CreateImage(name, description string, pip []string, conda *CondaValue, baseImage string, nvidia bool) (*Image, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/images")
	if err != nil {
		return nil, err
	}

	body := ImageCreateRequest{
		Name:  name,
		Pip:   pip,
		Conda: conda,
	}

	if description != "" {
		body.Description = &description
	}

	if baseImage != "" {
		body.BaseImage = &baseImage
	}

	if nvidia {
		body.Nvidia = true
	}

	var result Image

	err = doJSON(http.MethodPost, endpoint, body, "create image", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListImages returns a paginated slice of images. The API returns a
// DataPage envelope; results are newest first.
func ListImages(offset, limit int) ([]ImageSummary, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/images")
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

	var page DataPage[ImageSummary]

	err = doJSON(http.MethodGet, endpoint, nil, "images", &page)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}

// ImageLogsResponse mirrors PipelineImageLogsResponse from the API.
type ImageLogsResponse struct {
	Logs string `json:"logs"`
}

// GetImage fetches a single image by ID from GET /api/v2/pipelines/images/{image_id}.
func GetImage(imageID string) (*Image, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/images/" + imageID)
	if err != nil {
		return nil, err
	}

	var result Image

	err = doJSON(http.MethodGet, endpoint, nil, "get image", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetImageBuildLogs fetches build logs for a specific image version from
// GET /api/v2/pipelines/images/{image_id}/versions/{version_id}/logs.
func GetImageBuildLogs(imageID string, version int) (*ImageLogsResponse, error) {
	endpoint, err := config.GetEndpointURL(
		"/api/v2/pipelines/images/" + imageID + "/versions/" + strconv.Itoa(version) + "/logs",
	)
	if err != nil {
		return nil, err
	}

	var result ImageLogsResponse

	err = doJSON(http.MethodGet, endpoint, nil, "image build logs", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateImage PATCHes an image with a new complete definition, creating a
// new immutable version. The supplied definition is a full replacement.
// The response includes the full Image with all versions ordered newest-first.
//
// The API requires the image name in the body; UpdateImage fetches it
// first so callers only need to supply the image ID.
func UpdateImage(imageID string, pip []string, conda *CondaValue, baseImage string, nvidia bool) (*Image, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/images/" + imageID)
	if err != nil {
		return nil, err
	}

	// Fetch the current image to resolve its canonical name — required by
	// the update request body even though the server overrides it with the
	// stored name anyway.
	current, err := GetImage(imageID)
	if err != nil {
		return nil, err
	}

	body := ImageUpdateRequest{
		Name:  current.Name,
		Pip:   pip,
		Conda: conda,
	}

	if baseImage != "" {
		body.BaseImage = &baseImage
	}

	if nvidia {
		body.Nvidia = true
	}

	var result Image

	err = doJSON(http.MethodPatch, endpoint, body, "update image", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteImage soft-deletes the most-recent active version of an image.
// If no active versions remain, the parent image is soft-deleted as well.
func DeleteImage(imageID string) error {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/images/" + imageID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete image")
}

// DeleteImageVersion soft-deletes a specific version of an image without
// touching the parent.
func DeleteImageVersion(imageID string, version int) error {
	endpoint, err := config.GetEndpointURL(
		"/api/v2/pipelines/images/" + imageID + "/versions/" + strconv.Itoa(version),
	)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete image version")
}
