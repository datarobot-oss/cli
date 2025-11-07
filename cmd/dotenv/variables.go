// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/misc/regexp2"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

type variable struct {
	name        string
	value       string
	description string
	secret      bool
	changed     bool
	commented   bool
}

type Variables []variable

func newFromLine(line string) variable {
	expr := regexp.MustCompile(`^(?P<commented>\s*#\s*)?(?P<name>[a-zA-Z_]+[a-zA-Z0-9_]*) *= *(?P<value>[^\n]*)\n$`)
	result := regexp2.NamedStringMatches(expr, line)

	return variable{
		name:      result["name"],
		value:     result["value"],
		secret:    knownVariables[result["name"]].secret,
		commented: result["commented"] != "",
	}
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

func handleExtraEnvVars(variables Variables) bool {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		log.Fatalf("Error determining repo root: %v", err)
	}

	userPrompts, _, err := envbuilder.GatherUserPrompts(repoRoot)
	if err != nil {
		log.Fatalf("Error gathering user prompts: %v", err)
	}

	// Create a new empty string set
	existingEnvVarsSet := make(map[string]struct{})
	// Add elements to the set
	for _, value := range variables {
		existingEnvVarsSet[value.name] = struct{}{}
	}

	extraEnvVarsFound := false

	for _, up := range userPrompts {
		_, exists := existingEnvVarsSet[up.Env]
		// If we have an Env Var we don't yet know about account for it
		if !exists {
			extraEnvVarsFound = true
			// Add it to set
			existingEnvVarsSet[up.Env] = struct{}{}
			// Add it to variables
			variables = append(variables, variable{name: up.Env, value: up.Default, description: up.Help})
		}
	}

	if extraEnvVarsFound {
		fmt.Println("Environment Configuration")
		fmt.Println("=========================")
		fmt.Println("")
		fmt.Println("Editing .env file with component-specific variables...")
		fmt.Println("")

		for _, up := range userPrompts {
			if !up.HasEnvValue() {
				continue
			}

			style := tui.ErrorStyle

			if up.Valid() {
				style = tui.BaseTextStyle
			}

			fmt.Println(style.Render(up.StringWithoutHelp()))
		}

		fmt.Println("")
		fmt.Println("Configure required missing variables now? (y/N): ")

		reader := bufio.NewReader(os.Stdin)

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Error reading user reply: %v", err)
		}

		return strings.ToLower(strings.TrimSpace(selectedOption)) == "y"
	}

	return false
}
