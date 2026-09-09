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

// Delete sends body as JSON via HTTP DELETE. timeout optionally overrides
// DefaultClientTimeout, the same seam Get/Post/Patch have. Omitted, or ≤0,
// falls back to DefaultClientTimeout (see NewHTTPClient).
func Delete(url, info string, body any, timeout ...time.Duration) (*http.Response, error) {
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	if err = AuthorizeRequest(req); err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if info != "" {
		log.Debugf("Deleting %s at: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	if err := restoreRequestBody(req); err != nil {
		return nil, err
	}

	resp, err := NewHTTPClient(t).Do(req)
	if err != nil {
		return nil, err
	}

	if !isDeleteSuccess(resp.StatusCode) {
		// Use ErrFromResp (as Post does) so delete failures carry the
		// server's detail: workloads DELETE replies 502 with a "please
		// retry" hint when proton cleanup fails, and artifacts DELETE
		// replies 409 naming the workloads that block the deletion.
		return nil, ErrFromResp(resp, url)
	}

	return resp, err
}

func isDeleteSuccess(code int) bool {
	return code == http.StatusOK || code == http.StatusAccepted || code == http.StatusNoContent
}

// DeleteJSON is Delete with the response body decoded into v. timeout forwards to Delete.
func DeleteJSON(url, info string, body, v any, timeout ...time.Duration) error {
	resp, err := Delete(url, info, body, timeout...)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// 204 has no body — decoding returns io.EOF and would mask a
	// successful delete. Delete() already accepts 204 as success.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if v == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
