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
	Detail     string
}

// Error implements the error interface for HTTPError.
func (e *HTTPError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("HTTP %d %s: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Detail)
	}

	return fmt.Sprintf("HTTP error: %d %s (url: %s)", e.StatusCode, http.StatusText(e.StatusCode), e.URL)
}

var token string

// GetToken returns the current cached API token.
func GetToken() string {
	return token
}

// SetToken sets the cached API token.
func SetToken(value string) {
	token = value
}

// resolveToken returns the API token used for outbound requests.
// When --skip-auth (or DATAROBOT_CLI_SKIP_AUTH) is active we trust whatever
// is in viper without contacting the server, so local development against
// stub APIs that don't implement /version/ still works.
func resolveToken() (string, error) {
	if viperx.GetBool("skip_auth") {
		return viperx.GetString(config.DataRobotAPIKey), nil
	}

	return config.GetAPIKey(context.Background())
}

func Get(url, info string, timeoutSecs ...int) (*http.Response, error) {
	timeout := DefaultClientTimeout
	if len(timeoutSecs) > 0 {
		timeout = time.Duration(timeoutSecs[0]) * time.Second
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if err = AuthorizeRequest(req); err != nil {
		return nil, err
	}

	if info != "" {
		log.Infof("Fetching %s from: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	resp, err := NewHTTPClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()

		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: url}
	}

	return resp, err
}

func GetJSON(url, info string, v any, timeoutSecs ...int) error {
	resp, err := Get(url, info, timeoutSecs...)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(&v)
}
