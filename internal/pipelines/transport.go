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

// transport.go centralizes the request execution paths used by the
// non-multipart pipelines endpoints (inputs, runs, schedules):
//
//   - doJSON  - POST/PATCH a JSON body, optionally decode response.
//   - doDelete - DELETE returning 204 (or any 2xx with empty body).
//
// Authorization, User-Agent, and consumer-trace headers are owned by the
// shared drapi.AuthorizeRequest helper so the headers stay consistent with
// every other CLI command.

package pipelines

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
)

// jsonTimeout is used for JSON request/response endpoints. These should
// finish quickly compared to multipart uploads, so we keep the default
// drapi timeout.
const jsonTimeout = drapi.DefaultClientTimeout

// doJSON performs a request with a JSON-encoded body. If body is nil the
// request is sent with no body (useful for status-only POSTs). If out is
// nil the response body is discarded.
func doJSON(method, endpoint string, body any, info string, out any) error {
	req, err := buildJSONRequest(method, endpoint, body)
	if err != nil {
		return err
	}

	if info != "" {
		log.Infof("%s at: %s", info, endpoint)
	}

	if log.GetLevel() <= log.DebugLevel {
		log.Debug("Request Info: \n" + config.RedactedReqInfo(req))
	}

	client := drapi.NewHTTPClient(jsonTimeout)

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

// buildJSONRequest assembles an authenticated *http.Request with a
// JSON-encoded body. Extracted from doJSON to keep doJSON's cyclomatic
// complexity within lint limits.
func buildJSONRequest(method, endpoint string, body any) (*http.Request, error) {
	reqBody := &bytes.Buffer{}

	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		reqBody = bytes.NewBuffer(buf)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	err = drapi.AuthorizeRequest(req)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// doDelete sends a DELETE and treats any 2xx response as success. The
// response body is drained but ignored.
func doDelete(endpoint, info string) error {
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	err = drapi.AuthorizeRequest(req)
	if err != nil {
		return err
	}

	if info != "" {
		log.Infof("%s at: %s", info, endpoint)
	}

	if log.GetLevel() <= log.DebugLevel {
		log.Debug("Request Info: \n" + config.RedactedReqInfo(req))
	}

	client := drapi.NewHTTPClient(jsonTimeout)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp, endpoint)
	}

	return nil
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
