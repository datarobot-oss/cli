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
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
)

// Patch sends body as JSON via HTTP PATCH. timeout optionally overrides
// DefaultClientTimeout, the same seam Get/Post have. Omitted, or ≤0, falls
// back to DefaultClientTimeout (see NewHTTPClient).
func Patch(url, info string, body any, timeout ...time.Duration) (*http.Response, error) {
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	if err = AuthorizeRequest(req); err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if info != "" {
		log.Debugf("Updating %s at: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	if err := restoreRequestBody(req); err != nil {
		return nil, err
	}

	resp, err := NewHTTPClient(t).Do(req)
	if err != nil {
		return nil, err
	}

	if !isPatchSuccess(resp.StatusCode) {
		// Use ErrFromResp (as Post and Delete do) so patch failures carry
		// the server's error detail instead of a bare status code.
		return nil, ErrFromResp(resp, url)
	}

	return resp, err
}

// isPatchSuccess accepts 202 as well as 200 and 204. A route that starts work
// rather than finishing it answers Accepted with a handle to follow, which the
// workload settings route does: it applies new sizing by rolling a
// replacement. Reading that as a failure would report every accepted change as
// broken while the platform got on with it.
func isPatchSuccess(code int) bool {
	return code == http.StatusOK || code == http.StatusAccepted || code == http.StatusNoContent
}

// PatchJSON is Patch with the response body decoded into v. timeout forwards to Patch.
func PatchJSON(url, info string, body, v any, timeout ...time.Duration) error {
	resp, err := Patch(url, info, body, timeout...)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// 204 has no body — decoding returns io.EOF and would mask a
	// successful patch. Patch() already accepts 204 as success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if v == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
