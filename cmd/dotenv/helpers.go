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
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

func generateRandomSecret(length int) (string, error) {
	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("Failed to generate random bytes: %w", err)
	}

	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

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
