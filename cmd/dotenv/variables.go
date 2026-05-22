// Copyright 2026 DataRobot, Inc. and its affiliates.
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
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

// handleExtraEnvVars detects and prompts the user to configure component-specific environment variables.
// It gathers user prompts from the application template configuration and compares them against
// the provided variables. If extra variables are found (defined in the template but not in variables),
// it displays them and prompts the user to configure them interactively.
//
// Parameters:
//   - variables: the current environment variables already set or parsed from .env
//
// Returns:
//   - bool: true if user answered 'y' to configure the extra variables (launch wizard), false otherwise
//   - error: if determining repo root, gathering prompts, or reading user input fails
func handleExtraEnvVars(variables envbuilder.Variables) (bool, error) { //nolint: cyclop
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		return false, fmt.Errorf("error determining repo root: %w", err)
	}

	userPrompts, err := envbuilder.GatherUserPrompts(repoRoot, variables)
	if err != nil {
		return false, fmt.Errorf("error gathering user prompts: %w", err)
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
			return false, fmt.Errorf("error reading user reply: %w", err)
		}

		return strings.ToLower(strings.TrimSpace(selectedOption)) == "y", nil
	}

	return false, nil
}
