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

package outputformat

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/datarobot/cli/internal/drapi"
)

// ErrorBody is the value under "error" in the JSON error envelope. The HTTP
// fields are set only when the error chain carries a *drapi.HTTPError.
type ErrorBody struct {
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode,omitempty"`
	URL        string `json:"url,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// NewErrorBody builds the envelope body for err.
func NewErrorBody(err error) ErrorBody {
	body := ErrorBody{Message: err.Error()}

	var httpErr *drapi.HTTPError
	if errors.As(err, &httpErr) {
		body.StatusCode = httpErr.StatusCode
		body.URL = httpErr.URL
		body.Detail = httpErr.Detail
	}

	return body
}

// PrintJSONError writes {"error": {...}} for err to w as a single line, so a
// caller can pick it out of a stderr shared with warnings.
func PrintJSONError(w io.Writer, err error) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	return enc.Encode(map[string]ErrorBody{"error": NewErrorBody(err)})
}
