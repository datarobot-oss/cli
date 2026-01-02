// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// VerifyToken verifies if the datarobot host + api key pair correspond to a valid pair.
func VerifyToken(datarobotHost, token string) error {
	if token == "" {
		return errors.New("empty token")
	}

	req, err := http.NewRequest(http.MethodGet, datarobotHost+"/api/v2/version/", nil)
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
	datarobotHost := GetBaseURL()

	// Return API key from environment variable
	envToken := os.Getenv("DATAROBOT_API_TOKEN")
	if err := VerifyToken(datarobotHost, envToken); err == nil {
		return envToken, nil
	}

	// Return API key from viper config
	viperToken := viper.GetString(DataRobotAPIKey)

	err := VerifyToken(datarobotHost, viperToken)
	if err != nil {
		return "", err
	}

	return viperToken, nil
}
