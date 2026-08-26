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

// Package apiclient is the Workload API's own client layer over drapi.
//
// drapi serves every DataRobot API the CLI talks to, and those APIs do not
// agree on an error envelope: the Workload API and its sibling FastAPI
// services answer {"detail": ...}, the drflask-based Public API answers
// {"message": ...}, and a proxy in between answers plain text. drapi
// therefore carries a failed response's body raw and uninterpreted, and this
// package is where the Workload API's envelope is read into meaning. The
// same split exists for pipelines, whose client layer is internal/pipeline.
package apiclient

import (
	"encoding/json"
	"errors"

	"github.com/datarobot/cli/internal/drapi"
)

// PostJSON is drapi.PostJSON with the Workload API's error envelope applied:
// a failure whose body is a {"detail": ...} document is rewrapped so the
// caller reads the server's own sentence instead of a raw JSON dump. Every
// other error passes through untouched.
func PostJSON(url, info string, body, v any, timeoutSecs ...int) error {
	return LiftDetail(drapi.PostJSON(url, info, body, v, timeoutSecs...))
}

// LiftDetail applies the Workload API's error envelope to an error minted by
// drapi. When the error is an HTTPError whose raw body carries a FastAPI
// detail, the detail becomes the error's message; anything else — nil, a
// non-HTTP error, a body in some other shape — is returned as it came.
func LiftDetail(err error) error {
	var httpErr *drapi.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	detail := ErrorDetail(httpErr.Body)
	if detail == "" {
		return err
	}

	return &drapi.HTTPError{
		StatusCode: httpErr.StatusCode,
		URL:        httpErr.URL,
		Detail:     detail,
		Body:       httpErr.Body,
	}
}

// ErrorDetail pulls the "detail" field out of a FastAPI-style JSON error
// body, or "" when the body is not such a document. Non-string details
// (e.g. validation error arrays) are re-encoded as JSON rather than dropped:
// they carry the field-level messages the caller needs.
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
