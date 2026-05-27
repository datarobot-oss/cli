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

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
)

func Patch(url, info string, body any) (*http.Response, error) {
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
		log.Infof("Updating %s at: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	if err := restoreRequestBody(req); err != nil {
		return nil, err
	}

	resp, err := NewHTTPClient(DefaultClientTimeout).Do(req)
	if err != nil {
		return nil, err
	}

	if !isPatchSuccess(resp.StatusCode) {
		resp.Body.Close()

		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: url}
	}

	return resp, err
}

func isPatchSuccess(code int) bool {
	return code == http.StatusOK || code == http.StatusNoContent
}

func PatchJSON(url, info string, body, v any) error {
	resp, err := Patch(url, info, body)
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
