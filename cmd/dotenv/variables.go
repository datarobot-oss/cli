// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"regexp"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/spf13/viper"
)

type variable struct {
	name      string
	value     string
	secret    bool
	changed   bool
	commented bool
}

func newFromLine(line string) variable {
	expr := regexp.MustCompile(`^(?P<commented>\s*#\s*)?(?P<name>[a-zA-Z_]+[a-zA-Z0-9_]*) *= *(?P<value>[^\n]*)\n$`)
	result := regexp2.NamedStringMatches(expr, line)
	v := variable{}

	v.name = result["name"]
	v.value = result["value"]

	if result["commented"] != "" {
		v.commented = true
	}

	if knownVariables[v.name].secret {
		v.secret = true
	}

	return v
}

func (v *variable) String() string {
	if v.commented {
		return "# " + v.name + "=" + v.value + "\n"
	}

	return v.name + "=" + v.value + "\n"
}

func (v *variable) setValue() {
	conf, found := knownVariables[v.name]

	if !found {
		return
	}

	oldValue := v.value

	switch {
	case conf.viperKey != "":
		v.value = viper.GetString(conf.viperKey)
	case conf.getValue != nil:
		var err error

		v.value, err = conf.getValue()
		if err != nil && v.value != "" {
			// Only log error if we actually got a non-empty value with an error
			// Ignore "empty url" and similar errors when exiting setup
			log.Error(err)
		}
	}

	if v.value != oldValue {
		v.changed = true
	}
}

type variableConfig = struct {
	viperKey string
	getValue func() (string, error)
	secret   bool
}

// knownVariables maps well-known environment variable names to their configurations

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
