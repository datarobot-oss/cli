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
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
)

func Post(url, info string, body any) (*http.Response, error) {
	var err error

	if token == "" {
		token, err = config.GetAPIKey(context.Background())
		if err != nil {
			return nil, err
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("User-Agent", config.GetUserAgentHeader())
	req.Header.Add("Content-Type", "application/json")

	if config.IsAPIConsumerTrackingEnabled() {
		req.Header.Add("X-DataRobot-Api-Consumer-Trace", config.GetAPIConsumerTrace())
	}

	if info != "" {
		log.Infof("Creating %s at: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	if err := restoreRequestBody(req); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if !isCreateSuccess(resp.StatusCode) {
		resp.Body.Close()

		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: url}
	}

	return resp, err
}

func isCreateSuccess(code int) bool {
	return code == http.StatusOK || code == http.StatusCreated
}

// restoreRequestBody re-arms req.Body after RedactedReqInfo (which dumps and
// consumes it). For *bytes.Reader payloads, http.NewRequest sets req.GetBody
// automatically; for other body kinds we leave Body alone and rely on the
// transport reading whatever is left.
func restoreRequestBody(req *http.Request) error {
	if req.GetBody == nil {
		return nil
	}

	body, err := req.GetBody()
	if err != nil {
		return err
	}

	req.Body = body

	return nil
}

func PostJSON(url, info string, body, v any) error {
	resp, err := Post(url, info, body)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(&v)
}
