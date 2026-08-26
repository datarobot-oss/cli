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

package drapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/datarobot/cli/internal/config"
)

// EndpointURL returns a full DataRobot API URL for the given v2 path,
// optionally with the supplied query parameters appended. The path
// argument should start with "/" and is joined under "/api/v2"; e.g.
// EndpointURL("/files/", q) yields "<base>/api/v2/files/?<q>".
func EndpointURL(path string, query url.Values) (string, error) {
	full, err := config.GetEndpointURL("/api/v2" + path)
	if err != nil {
		return "", err
	}

	if len(query) == 0 {
		return full, nil
	}

	return full + "?" + query.Encode(), nil
}

// ErrFromResp wraps a non-2xx *http.Response into a *HTTPError, capturing
// up to 512 bytes of the response body for context. It always closes
// resp.Body, so callers must not read it after calling this function.
//
// When the body is a FastAPI-style `{"detail": ...}` document, the detail is
// lifted into HTTPError.Detail so the user reads the server's own sentence
// instead of a raw JSON dump; anything else keeps the body= form, whose URL
// is what makes an unstructured failure diagnosable.
func ErrFromResp(resp *http.Response, requestURL string) error {
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if detail := ErrorDetail(body); detail != "" {
		return &HTTPError{StatusCode: resp.StatusCode, URL: requestURL, Detail: detail}
	}

	if len(body) > 0 {
		return fmt.Errorf("%w: body=%s", &HTTPError{StatusCode: resp.StatusCode, URL: requestURL}, body)
	}

	return &HTTPError{StatusCode: resp.StatusCode, URL: requestURL}
}

// ErrorDetail pulls the "detail" field out of a FastAPI-style JSON error
// body, or "" when the body is not such a document. Non-string details
// (e.g. validation error arrays) are re-encoded as JSON rather than dropped:
// they carry the field-level messages the caller needs.
//
// This is the one place that knows the server's error envelope; callers that
// want a different fallback (e.g. the raw body) layer it on the "" return.
func ErrorDetail(body []byte) string {
	var payload struct {
		Detail any `json:"detail"`
	}

	if err := json.Unmarshal(body, &payload); err != nil || payload.Detail == nil {
		return ""
	}

	if detail, ok := payload.Detail.(string); ok {
		return detail
	}

	encoded, err := json.Marshal(payload.Detail)
	if err != nil {
		return ""
	}

	return string(encoded)
}
