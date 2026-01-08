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
