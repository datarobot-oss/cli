// Copyright 2025 DataRobot, Inc. and its affiliates.
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
	"errors"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
)

var token string

func Get(url, info string) (*http.Response, error) {
	var err error

	// memoize token to avoid extra VerifyToken() calls
	if token == "" {
		token, err = config.GetAPIKey()
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("User-Agent", config.GetUserAgentHeader())

	if info != "" {
		log.Infof("Fetching %s from: %s", info, url)
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, errors.New("Response status code is " + resp.Status + ".")
	}

	return resp, err
}

func GetJSON(url, info string, v any) error {
	resp, err := Get(url, info)
	if err != nil {
		return err
	}

	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		return err
	}

	resp.Body.Close()

	return nil
}
