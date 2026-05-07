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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

// ensureInRepo checks if we're in a git repository, and returns the repo root path.
func ensureInRepo() (string, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil {
		fmt.Println(tui.ErrorStyle.Render("Oops! ") + "This command needs to run inside your AI application folder.")
		fmt.Println()
		fmt.Println("📁 What this means:")
		fmt.Println("   You need to be in a folder that contains your AI application code.")
		fmt.Println()
		fmt.Println("🔧 How to fix this:")
		fmt.Println("   1. If you haven't created an app yet: run " + tui.InfoStyle.Render("dr templates setup"))
		fmt.Println("   2. If you have an app: navigate to its folder using " + tui.InfoStyle.Render("cd your-app-name"))
		fmt.Println("   3. Then try this command again")

		return "", errors.New("Not in git repository.")
	}

	return repoRoot, nil
}

// ensureInRepoWithDotenv checks if we're in a git repository and if .env file exists.
// It prints appropriate error messages and returns the dotenv file path if successful.
func ensureInRepoWithDotenv() (string, error) {
	repoRoot, err := ensureInRepo()
	if err != nil {
		return "", err
	}

	dotenv := filepath.Join(repoRoot, ".env")

	if _, err := os.Stat(dotenv); os.IsNotExist(err) {
		fmt.Printf("%s: Your app is missing its configuration file (.env)\n", tui.ErrorStyle.Render("Missing Config"))
		fmt.Println()
		fmt.Println("📄 What this means:")
		fmt.Println("   Your AI application needs a '.env' file to store settings like API keys.")
		fmt.Println()
		fmt.Println("🔧 How to fix this:")
		fmt.Println("   Run " + tui.InfoStyle.Render("dr dotenv setup") + " to create the configuration file.")
		fmt.Println("   This will guide you through setting up all required settings.")

		return "", errors.New("'.env' file does not exist.")
	}

	return dotenv, nil
}

// ValidateAndEditIfNeeded validates the .env file and prompts for editing if validation fails.
// Returns nil if validation passes or editing completes successfully.
// Returns an error if validation or editing fails.
func ValidateAndEditIfNeeded() error {
	dotenv, err := ensureInRepoWithDotenv()
	if err != nil {
		return err
	}

	repoRoot := filepath.Dir(dotenv)

	dotenvFileLines, contents := readDotenvFile(dotenv)

	// Parse variables from '.env' file
	parsedVars := envbuilder.ParseVariablesOnly(dotenvFileLines)

	// Validate using envbuilder
	result := envbuilder.ValidateEnvironment(repoRoot, parsedVars)

	// If validation passes, we're done
	if !result.HasErrors() {
		return nil
	}

	// Validation failed, prompt user to edit
	fmt.Println()
	fmt.Println(tui.InfoStyle.Render("⚠️  Configuration Update Needed"))
	fmt.Println()
	fmt.Println("The newly added component requires additional environment variables.")
	fmt.Println("Let's set those up now.")
	fmt.Println()

	// Check if there are extra variables that need wizard setup
	variables := envbuilder.ParseVariablesOnly(dotenvFileLines)
	screen := editorScreen

	if handleExtraEnvVars(variables) {
		screen = wizardScreen
	}

	// Launch the edit flow
	m := Model{
		initialScreen: screen,
		DotenvFile:    dotenv,
		variables:     variables,
		contents:      contents,
		SuccessCmd:    tea.Quit,
	}

	_, err = tui.Run(m, tea.WithAltScreen())
	if err != nil {
		fmt.Println()
		fmt.Println(tui.ErrorStyle.Render("⚠️  Configuration update incomplete"))
		fmt.Println()
		fmt.Println("You may need to update your '.env' file manually or run:")
		fmt.Println("  " + tui.InfoStyle.Render("dr dotenv edit"))
		fmt.Println()

		return err
	}

	return nil
}

// applyDefaultsToPrompts auto-populates prompts with their default values,
// applies generated values, and determines required sections. This is the
// core logic shared between interactive and non-interactive setup flows.
// Modifies the prompts slice in place and returns it (or error if generation fails).
func applyDefaultsToPrompts(prompts []envbuilder.UserPrompt) ([]envbuilder.UserPrompt, error) {
	// Auto-populate all prompts with their default values (or empty strings)
	for i := range prompts {
		prompt := &prompts[i]

		// Ensure variables are uncommented (consistent with interactive wizard)
		prompt.Commented = false

		// Skip if already has a value (e.g., from environment or existing .env)
		if prompt.Value != "" {
			continue
		}

		// Use default if available
		if prompt.Default != "" {
			prompt.Value = prompt.Default
		}
		// Otherwise leave as empty string (which is the zero value)
	}

	var err error

	// Apply generated values for prompts with field: `generate: true`
	prompts, err = envbuilder.ApplyGeneratedValues(prompts)
	if err != nil {
		return nil, err
	}

	// Determine required sections based on selected options
	prompts = envbuilder.DetermineRequiredSections(prompts)

	return prompts, nil
}

// setupNonInteractive performs non-interactive dotenv setup without launching the TUI.
// It auto-populates all prompts with their default values (or empty strings) and saves
// the .env file. This is used when --yes flag is set or DATAROBOT_CLI_NON_INTERACTIVE=true.
func setupNonInteractive(repositoryRoot, dotenvFile string) error {
	// Read existing .env file or use default template
	dotenvFileLines, contents := readDotenvFile(dotenvFile)
	variables, _ := envbuilder.VariablesFromLines(dotenvFileLines)

	// Check if Pulumi passphrase setup is needed and handle it before gathering prompts
	// (this will save the passphrase to drconfig.yaml if needed)
	if err := handlePulumiPassphraseNonInteractive(repositoryRoot, variables); err != nil {
		return fmt.Errorf("failed to setup Pulumi passphrase: %w", err)
	}

	// Gather user prompts from template configuration
	// (if passphrase was just saved, it will be picked up from drconfig.yaml via viper)
	prompts, err := envbuilder.GatherUserPrompts(repositoryRoot, variables)
	if err != nil {
		return fmt.Errorf("failed to gather prompts: %w", err)
	}

	// Apply defaults and process prompts (shared logic with TUI)
	prompts, err = applyDefaultsToPrompts(prompts)
	if err != nil {
		return fmt.Errorf("failed to process prompts: %w", err)
	}

	// Generate .env content from prompts merged with existing contents
	newContents := envbuilder.DotenvFromPromptsMerged(prompts, contents)

	// Save the file
	if err := writeContents(newContents, dotenvFile); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	return nil
}
