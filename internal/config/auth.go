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

package config

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// VerifyToken verifies if the datarobot endpoint + api key pair correspond to a valid pair.
func VerifyToken(datarobotEndpoint, token string) error {
	_, err := url.Parse(datarobotEndpoint)
	if err != nil {
		return err
	}

	if token == "" {
		return errors.New("empty token")
	}

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint+"/version/", nil)
	if err != nil {
		return err
	}

	bearer := "Bearer " + token
	req.Header.Add("Authorization", bearer)
	req.Header.Add("User-Agent", GetUserAgentHeader())

	log.Debug("Request Info: \n" + RedactedReqInfo(req))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("invalid token")
	}

	return nil
}

func GetAPIKey() (string, error) {
	viperEndpoint := viper.GetString(DataRobotURL)
	viperToken := viper.GetString(DataRobotAPIKey)

	// Returns valid API key if there is one, otherwise returns an empty string
	err := VerifyToken(viperEndpoint, viperToken)
	if err != nil {
		return "", err
	}

	return viperToken, nil
}
