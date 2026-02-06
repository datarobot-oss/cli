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

package dotenv

import (
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

func handleExtraEnvVars(variables envbuilder.Variables) bool { //nolint: cyclop
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		log.Fatalf("Error determining repo root: %v", err)
	}

	userPrompts, err := envbuilder.GatherUserPrompts(repoRoot, variables)
	if err != nil {
		log.Fatalf("Error gathering user prompts: %v", err)
	}

	// Create a new empty string set
	existingEnvVarsSet := make(map[string]struct{})
	// Add elements to the set
	for _, value := range variables {
		existingEnvVarsSet[value.Name] = struct{}{}
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
			variables = append(variables, envbuilder.Variable{Name: up.Env, Value: up.Default, Description: up.Help})
		}
	}

	if extraEnvVarsFound {
		fmt.Println("Environment Configuration")
		fmt.Println("=========================")
		fmt.Println("")
		fmt.Println("Editing '.env' file with component-specific variables...")
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

		selectedOption, err := reader.ReadString()
		if err != nil {
			log.Fatalf("Error reading user reply: %v", err)
		}

		return strings.ToLower(strings.TrimSpace(selectedOption)) == "y"
	}

	return false
}
