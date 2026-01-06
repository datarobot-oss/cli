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
