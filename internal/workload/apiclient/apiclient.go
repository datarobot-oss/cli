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
//
// Contract: every exported function here must keep returning something
// errors.As can unpack into *drapi.HTTPError — the original error unchanged,
// %w-wrapped, or (as LiftDetail's detail-found branch does) a fresh
// *drapi.HTTPError carrying the same StatusCode/URL. At least 32 call sites
// across cmd/pipeline, cmd/artifact, cmd/workload, cmd/dotenv,
// internal/workload, and internal/pipeline depend on errors.As(err, &httpErr)
// succeeding to read StatusCode. Breaking that chain is silent: no compile
// error, no red test — every one of those checks just starts returning false.
package apiclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/datarobot/cli/internal/drapi"
)

// PostJSON is drapi.PostJSON with the Workload API's error envelope applied:
// a failure whose body is a {"detail": ...} document is rewrapped so the
// caller reads the server's own sentence instead of a raw JSON dump. Every
// other error passes through untouched. timeout forwards to drapi.PostJSON.
func PostJSON(url, info string, body, v any, timeout ...time.Duration) error {
	return LiftDetail(drapi.PostJSON(url, info, body, v, timeout...))
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

// typedDetail is a typed rejection: the {"code", "message"} detail that
// workload placement errors use. Fields the server may add later are ignored.
type typedDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorDetail pulls the "detail" field out of a FastAPI-style JSON error
// body, or "" when the body is not such a document. A typed rejection
// renders as the message with the machine code in parentheses. Any other
// detail (e.g. a validation error array) is returned as compact JSON rather
// than dropped: it carries the field-level messages the caller needs.
func ErrorDetail(body []byte) string {
	var payload struct {
		Detail json.RawMessage `json:"detail"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	detail := bytes.TrimSpace(payload.Detail)
	if len(detail) == 0 || bytes.Equal(detail, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(detail, &text); err == nil {
		return text
	}

	var typed typedDetail
	if err := json.Unmarshal(detail, &typed); err == nil && typed.Code != "" && typed.Message != "" {
		return fmt.Sprintf("%s (%s)", typed.Message, typed.Code)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, detail); err != nil {
		return ""
	}

	return compact.String()
}
