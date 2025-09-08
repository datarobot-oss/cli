// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package drapi

import (
	"net/http"

	"github.com/datarobot/cli/internal/config"
)

func IsLLMGatewayEnabled() (bool, error) {
	datarobotEndpoint, err := config.GetEndpointURL("/api/v2/genai/llms/")
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint, nil)
	if err != nil {
		return false, err
	}

	bearer := "Bearer " + config.GetAPIKey()
	req.Header.Add("Authorization", bearer)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, nil
	}

	return false, nil
}
