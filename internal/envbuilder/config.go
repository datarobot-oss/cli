// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"github.com/datarobot/cli/internal/config"
)

type variableConfig = struct {
	value  string
	secret bool
}

func knownVariables(allValues map[string]string) map[string]variableConfig {
	datarobotEndpoint := allValues["DATAROBOT_ENDPOINT"]
	token := allValues["DATAROBOT_API_TOKEN"]

	err := config.VerifyToken(datarobotEndpoint, token)
	if err != nil {
		datarobotEndpoint, _ = config.GetEndpointURL("/api/v2")
		token, _ = config.GetAPIKey()
	}

	return map[string]variableConfig{
		"DATAROBOT_ENDPOINT": {
			value: datarobotEndpoint,
		},
		"DATAROBOT_API_TOKEN": {
			value:  token,
			secret: true,
		},
	}
}

const coreSection = "__DR_CLI_CORE_PROMPT"

var corePrompts = []UserPrompt{
	{
		Section: coreSection,
		Root:    true,
		Active:  true,
		Hidden:  true,

		Env:      "DATAROBOT_ENDPOINT",
		Type:     "string",
		Help:     "The URL of your DataRobot instance API.",
		Optional: false,
	},
	{
		Section: coreSection,
		Root:    true,
		Active:  true,
		Hidden:  true,

		Env:      "DATAROBOT_API_TOKEN",
		Type:     "string",
		Help:     "Refer to https://docs.datarobot.com/en/docs/api/api-quickstart/index.html#configure-your-environment for help.",
		Optional: false,
	},
}
