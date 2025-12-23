// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// VerifyToken verifies if the datarobot host + api key pair correspond to a valid pair.
func VerifyToken(datarobotHost, token string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, datarobotHost+"/api/v2/version/", nil)
	if err != nil {
		return false, err
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
		return false, err
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func GetAPIKey() string {
	datarobotHost := GetBaseURL()
	token := os.Getenv("DATAROBOT_API_TOKEN")

	if token != "" {
		if isValid, _ := VerifyToken(datarobotHost, token); isValid {
			return token
		}
	}

	// Returns the API key if there is one, otherwise returns an empty string
	token = viper.GetString(DataRobotAPIKey)
	if isValid, _ := VerifyToken(datarobotHost, token); isValid {
		return token
	}

	return ""
}
