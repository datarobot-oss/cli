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

package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
)

// uploadTimeout is the per-request timeout used for multipart file uploads.
const uploadTimeout = 60 * time.Second

// Mode values accepted by the pipelines API.
const (
	ModeDraft  = "draft"
	ModeLocked = "locked"
)

// PipelineVersion mirrors PipelineVersionResponse from the pipelines-api.
type PipelineVersion struct {
	Version        int            `json:"version"`
	Status         string         `json:"status"`
	TaskNames      []string       `json:"taskNames,omitempty"`
	PythonVersion  string         `json:"pythonVersion"`
	ResourceBundle map[string]any `json:"resourceBundle,omitempty"`
	ErrorDetail    string         `json:"errorDetail,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// Pipeline mirrors PipelineDetailResponse from the pipelines-api.
type Pipeline struct {
	PipelineID     string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Mode           string            `json:"mode"`
	IsActive       bool              `json:"isActive"`
	TaskNames      []string          `json:"taskNames,omitempty"`
	PythonVersion  string            `json:"pythonVersion,omitempty"`
	ResourceBundle map[string]any    `json:"resourceBundle,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	Versions       []PipelineVersion `json:"versions"`
}

// CreateResponse mirrors PipelineCreateResponse from the pipelines-api.
// It is also returned by PATCH /pipelines/{id}.
type CreateResponse struct {
	PipelineID string    `json:"id"`
	Name       string    `json:"name"`
	Version    int       `json:"version"`
	Status     string    `json:"status"`
	Mode       string    `json:"mode"`
	TaskNames  []string  `json:"taskNames,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ListItem mirrors PipelineListItem from the pipelines-api.
type ListItem struct {
	PipelineID    string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Mode          string    `json:"mode"`
	IsActive      bool      `json:"isActive"`
	LatestVersion *int      `json:"latestVersion,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CreatePipeline uploads a Python file to POST /api/v2/pipelines.
func CreatePipeline(filePath, description, mode string) (*CreateResponse, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines")
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	if description != "" {
		fields["description"] = description
	}

	if mode != "" {
		fields["mode"] = mode
	}

	var result CreateResponse

	err = doMultipart(http.MethodPost, endpoint, filePath, fields, "create pipeline", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListPipelines fetches a paginated list of pipelines from GET /api/v2/pipelines.
func ListPipelines(mode string, offset, limit int) (*DataPage[ListItem], error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if mode != "" {
		query.Set("mode", mode)
	}

	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}

	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}

	var page DataPage[ListItem]

	err = drapi.GetJSON(endpoint, "pipelines", &page)
	if err != nil {
		return nil, err
	}

	return &page, nil
}

// GetPipeline fetches a single pipeline from GET /api/v2/pipelines/{pipeline_id}.
func GetPipeline(pipelineID string) (*Pipeline, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID)
	if err != nil {
		return nil, err
	}

	var pipeline Pipeline

	err = drapi.GetJSON(endpoint, "pipeline", &pipeline)
	if err != nil {
		return nil, err
	}

	return &pipeline, nil
}

// UpdatePipeline re-uploads a Python file to PATCH /api/v2/pipelines/{pipeline_id}.
func UpdatePipeline(pipelineID, filePath string) (*CreateResponse, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID)
	if err != nil {
		return nil, err
	}

	var result CreateResponse

	err = doMultipart(http.MethodPatch, endpoint, filePath, nil, "update pipeline", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeletePipeline issues DELETE /api/v2/pipelines/{pipeline_id}. The API
// returns 204 on success.
func DeletePipeline(pipelineID string) error {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete pipeline")
}

// LockPipeline issues PATCH /api/v2/pipelines/{pipeline_id}/mode to
// promote a draft pipeline into the locked mode. The response mirrors a
// create/update payload, with `mode` set to "locked" and `version`
// pointing at the locked version.
func LockPipeline(pipelineID string) (*CreateResponse, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID + "/mode")
	if err != nil {
		return nil, err
	}

	var result CreateResponse

	err = doJSON(http.MethodPatch, endpoint, nil, "lock pipeline", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// doMultipart performs a multipart/form-data request with a single "file" upload
// and optional form fields, decoding the JSON response into out.
func doMultipart(method, endpoint, filePath string, fields map[string]string, info string, out any) error {
	req, err := buildMultipartRequest(method, endpoint, filePath, fields)
	if err != nil {
		return err
	}

	if info != "" {
		log.Infof("%s at: %s", info, endpoint)
	}

	// Only build the redacted request dump when debug logging is enabled —
	// httputil.DumpRequestOut(req, true) drains req.Body, which silently
	// breaks PATCH/POST multipart requests by leaving them with
	// ContentLength=N and a 0-byte body.
	if log.GetLevel() <= log.DebugLevel {
		log.Debug("Request Info: \n" + config.RedactedReqInfo(req))
	}

	client := drapi.NewHTTPClient(uploadTimeout)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp, endpoint)
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// buildMultipartRequest assembles the multipart body and HTTP request with
// authentication and tracing headers populated via drapi.AuthorizeRequest.
func buildMultipartRequest(method, endpoint, filePath string, fields map[string]string) (*http.Request, error) {
	body, contentType, err := buildMultipartBody(filePath, fields)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, err
	}

	// Authorization, User-Agent, and consumer-trace are owned by drapi so
	// every CLI command sends consistent headers.
	err = drapi.AuthorizeRequest(req)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	return req, nil
}

// decodeHTTPError reads a non-2xx response body and turns it into a meaningful error.
func decodeHTTPError(resp *http.Response, endpoint string) error {
	respBody, _ := io.ReadAll(resp.Body)

	detail := extractErrorDetail(respBody)
	if detail != "" {
		return fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), detail)
	}

	return &drapi.HTTPError{StatusCode: resp.StatusCode, URL: endpoint}
}

// buildMultipartBody constructs a multipart/form-data body containing the named
// file plus the given form fields.
func buildMultipartBody(filePath string, fields map[string]string) (*bytes.Buffer, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("open %s: %w", filePath, err)
	}

	defer file.Close()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, "", err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, "", err
	}

	for key, value := range fields {
		err = writer.WriteField(key, value)
		if err != nil {
			return nil, "", err
		}
	}

	err = writer.Close()
	if err != nil {
		return nil, "", err
	}

	return &body, writer.FormDataContentType(), nil
}

// extractErrorDetail attempts to pull a "detail" string from a JSON error body
// returned by FastAPI. Falls back to the raw body if the field is absent.
func extractErrorDetail(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var payload struct {
		Detail any `json:"detail"`
	}

	err := json.Unmarshal(body, &payload)
	if err == nil && payload.Detail != nil {
		switch detail := payload.Detail.(type) {
		case string:
			return detail
		default:
			encoded, encErr := json.Marshal(detail)
			if encErr == nil {
				return string(encoded)
			}
		}
	}

	return string(body)
}
