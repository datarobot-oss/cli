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
	viperKey string
	getValue func() (string, error)
	secret   bool
}

var knownVariables = map[string]variableConfig{
	"DATAROBOT_ENDPOINT_SHORT": {
		getValue: func() (string, error) {
			return config.GetEndpointURL("")
		},
	},
	"DATAROBOT_ENDPOINT": {
		getValue: func() (string, error) {
			return config.GetEndpointURL("/api/v2")
		},
	},
	"DATAROBOT_API_TOKEN": {
		getValue: func() (string, error) {
			return config.GetAPIKey(), nil
		},
		secret: true,
	},
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
