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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

// HTTPError is returned by Get when the server responds with a non-200 status code.
// Callers can extract the status code with errors.As to make decisions without string matching.
type HTTPError struct {
	StatusCode int
	URL        string

	// Detail is a best-effort, caller-parsed human-readable message (see
	// workload/apiclient.LiftDetail) — not a structural guarantee. It comes
	// back empty for reasons that have nothing to do with whether the
	// request actually failed: the API's envelope shape, a proxy in the
	// middle, a truncated body, even the caller's locale. Branch decisions
	// on StatusCode, which the server always sets; use Detail for display
	// only.
	Detail string

	// Body is up to 512 bytes of the error response, raw and uninterpreted.
	// DataRobot's APIs do not agree on an error envelope (FastAPI services
	// answer {"detail": ...}, the drflask Public API {"message": ...}, a
	// proxy plain text), so drapi carries the bytes and a caller that knows
	// its API's shape reads meaning into them — e.g. workload/apiclient.
	Body []byte
}

// Error implements the error interface for HTTPError. Both shapes carry the
// URL: a message like "Internal server error" identifies nothing without the
// endpoint it came from.
func (e *HTTPError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("HTTP %d %s: %s (url: %s)", e.StatusCode, http.StatusText(e.StatusCode), e.Detail, e.URL)
	}

	return fmt.Sprintf("HTTP error: %d %s (url: %s)", e.StatusCode, http.StatusText(e.StatusCode), e.URL)
}

// token is the cached API token. Every read and write takes tokenMu (declared
// in client.go), including the accessors below: GetLLMsAndDeployed fetches two
// endpoints at once, so a refresh path calling SetToken mid-fetch would race
// the readers.
var token string

// GetToken returns the current cached API token.
func GetToken() string {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	return token
}

// SetToken sets the cached API token.
func SetToken(value string) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	token = value
}

// resolveToken returns the API token used for outbound requests.
// When --skip-auth (or DATAROBOT_CLI_SKIP_AUTH) is active we trust whatever
// is in viper without contacting the server, so local development against
// stub APIs that don't implement /version/ still works.
func resolveToken() (string, error) {
	if viperx.GetBool(config.SkipAuthKey) {
		return viperx.GetString(config.DataRobotAPIKey), nil
	}

	return config.GetAPIKey(context.Background())
}

// Get sends a GET request. timeout optionally overrides DefaultClientTimeout;
// omitted, or ≤0, falls back to it (see NewHTTPClient).
func Get(url, info string, timeout ...time.Duration) (*http.Response, error) {
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if err = AuthorizeRequest(req); err != nil {
		return nil, err
	}

	if info != "" {
		log.Debugf("Fetching %s from: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	resp, err := NewHTTPClient(t).Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()

		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: url}
	}

	return resp, err
}

// GetJSON is Get with the response body decoded into v. timeout forwards to Get.
func GetJSON(url, info string, v any, timeout ...time.Duration) error {
	resp, err := Get(url, info, timeout...)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(&v)
}
