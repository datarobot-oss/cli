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
