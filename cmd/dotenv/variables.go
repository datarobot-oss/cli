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
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

func handleExtraEnvVars(variables envbuilder.Variables) bool {
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
