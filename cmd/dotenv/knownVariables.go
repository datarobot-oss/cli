// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"net/url"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

type variableConfig = struct {
	viperKey string
	getValue func() (string, error)
	secret   bool
}

var knownVariables = map[string]variableConfig{
	"DATAROBOT_ENDPOINT_SHORT": {
		viperKey: config.DataRobotURL,
	},
	"DATAROBOT_ENDPOINT": {
		getValue: func() (string, error) {
			fullURL, err := url.JoinPath(config.GetBaseURL(), "/api/v2")
			return fullURL, err
		},
	},
	"DATAROBOT_API_TOKEN": {
		viperKey: config.DataRobotAPIKey,
		secret:   true,
	},
	"USE_DATAROBOT_LLM_GATEWAY": {
		getValue: func() (string, error) {
			enabled, err := drapi.IsLLMGatewayEnabled()
			if err != nil {
				return "", err
			}

			if enabled {
				return "true", nil
			}
			return "false", nil
		},
	},
}
