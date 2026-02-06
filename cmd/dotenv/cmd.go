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
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dotenv",
		GroupID: "core",
		Short:   "🔧 Environment configuration commands.",
		Long: `Environment configuration commands for managing your application settings.

Manage your '.env' file and application configuration:
  • Edit environment variables interactively
  • Set up configuration with a guided wizard
  • Update DataRobot credentials automatically

🎯 Your '.env' file contains API keys, database connections, and other settings
   your application needs to run properly.`,
	}

	cmd.AddCommand(
		EditCmd,
		SetupCmd,
		UpdateCmd,
		ValidateCmd,
	)

	return cmd
}

var EditCmd = &cobra.Command{
	Use:   "edit",
	Short: "✏️ Edit '.env' file using built-in editor.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		dotenvFile := filepath.Join(cwd, ".env")
		dotenvFileLines, contents := readDotenvFile(dotenvFile)
		// Use ParseVariablesOnly to avoid auto-populating values during manual editing
		variables := envbuilder.ParseVariablesOnly(dotenvFileLines)

		// Default is editor screen but if we detect other Env Vars we'll potentially use wizard screen
		screen := editorScreen
		if repo.IsInRepo() {
			if handleExtraEnvVars(variables) {
				screen = wizardScreen
			}
		}

		m := Model{
			initialScreen: screen,
			DotenvFile:    dotenvFile,
			variables:     variables,
			contents:      contents,
			SuccessCmd:    tea.Quit,
		}
		_, err = tui.Run(m, tea.WithAltScreen(), tea.WithContext(cmd.Context()))

		return err
	},
}

var SetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "🧙 Environment configuration wizard.",
	Long: `Launch the interactive environment configuration wizard.

This wizard will help you:
  1️⃣  Review required environment variables
  2️⃣  Configure API keys and credentials
  3️⃣  Set up database connections (if needed)
  4️⃣  Validate your configuration

💡 Perfect for first-time setup or when adding new integrations.`,
	PreRunE: auth.EnsureAuthenticatedE,
	RunE: func(cmd *cobra.Command, _ []string) error {
		repositoryRoot, err := ensureInRepo()
		if err != nil {
			return err
		}

		dotenvFile := filepath.Join(repositoryRoot, ".env")

		// Check if we should skip when .env exists and all required variables are set
		flagIfNeededSet, _ := cmd.Flags().GetBool("if-needed")
		if flagIfNeededSet {
			shouldSkipSetup, err := shouldSkipSetup(repositoryRoot, dotenvFile)
			if err != nil {
				return err
			}

			if shouldSkipSetup {
				fmt.Println("Configuration already exists, skipping setup.")
				return nil
			}
		}

		// TODO: There's an inconsistency between validation and wizard variable loading:
		// - shouldSkipSetup uses ParseVariablesOnly (reads only .env file)
		// - ValidateEnvironment also checks OS environment variables (os.LookupEnv)
		// - But here we use VariablesFromLines which auto-populates from auth (setValue)
		//
		// This means:
		// 1. If validation passes (vars in .env or OS env), setup is skipped correctly
		// 2. If validation fails (vars missing), wizard runs but shows auto-populated values from auth
		//
		// This is probably acceptable UX (pre-fill makes wizard easier) but creates confusion
		// about what --if-needed is actually checking. Consider refactoring to be more consistent
		// or documenting the behavior more clearly in the flag description.
		dotenvFileLines, _ := readDotenvFile(dotenvFile)
		variables, contents := envbuilder.VariablesFromLines(dotenvFileLines)

		m := Model{
			initialScreen: wizardScreen,
			DotenvFile:    dotenvFile,
			variables:     variables,
			contents:      contents,
			SuccessCmd:    tea.Quit,
		}
		_, err = tui.Run(m, tea.WithAltScreen(), tea.WithContext(cmd.Context()))
		if err != nil {
			return err
		}

		// Update state after successful completion
		_ = state.UpdateAfterDotenvSetup(repositoryRoot)

		return nil
	},
}

func init() {
	SetupCmd.Flags().Bool("if-needed", false, "Only run setup if '.env' file doesn't exist or there are missing env vars.")
}

// shouldSkipSetup checks if setup should be skipped when --if-needed flag is set.
// Returns true if .env file exists and all required variables are valid.
//
// Note: This uses ParseVariablesOnly to read only what's in the .env file, but
// ValidateEnvironment also checks OS environment variables via os.LookupEnv.
// This means validation can pass if required variables are set as environment
// variables even if they're not in the .env file. This is intentional - if the
// app can run (because vars are available from any source), setup can be skipped.
func shouldSkipSetup(repositoryRoot, dotenvFile string) (bool, error) {
	if _, err := os.Stat(dotenvFile); err != nil {
		// .env doesn't exist, don't skip
		return false, nil
	}

	dotenvFileLines, _ := readDotenvFile(dotenvFile)
	variables := envbuilder.ParseVariablesOnly(dotenvFileLines)

	result := envbuilder.ValidateEnvironment(repositoryRoot, variables)

	return !result.HasErrors(), nil
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "🔄 Automatically update DataRobot credentials.",
	Long: `Automatically update your '.env' file with fresh DataRobot credentials.

This command will:
  • Refresh your DataRobot API credentials
  • Update environment variables automatically
  • Preserve your existing custom settings

💡 Use this when your credentials expire or you need to refresh your connection.`,
	PreRunE: auth.EnsureAuthenticatedE,
	Run: func(_ *cobra.Command, _ []string) {
		dotenv, err := ensureInRepoWithDotenv()
		if err != nil {
			os.Exit(1)
		}

		_, _, err = updateDotenvFile(dotenv)
		if err != nil {
			log.Error(err)
			os.Exit(1)
		}
	},
}

var ValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate '.env' and environment variable configuration against required settings.",
	Run: func(_ *cobra.Command, _ []string) {
		dotenv, err := ensureInRepoWithDotenv()
		if err != nil {
			os.Exit(1)
		}

		repoRoot := filepath.Dir(dotenv)

		dotenvFileLines, _ := readDotenvFile(dotenv)

		// Parse variables from '.env' file
		parsedVars := envbuilder.ParseVariablesOnly(dotenvFileLines)

		// Validate using envbuilder
		result := envbuilder.ValidateEnvironment(repoRoot, parsedVars)

		// Display results with styling
		varStyle := lipgloss.NewStyle().Foreground(tui.DrPurple).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(tui.DrGreen)

		// First, show all valid variables
		fmt.Println("\nValidating required variables:")

		for _, valResult := range result.Results {
			if valResult.Valid {
				fmt.Printf("  %s: %s\n",
					varStyle.Render(valResult.Field),
					valueStyle.Render(valResult.Value))
			}
		}

		// Then, show errors if any
		if result.HasErrors() {
			fmt.Println("\nValidation errors:")

			for _, valResult := range result.Results {
				if !valResult.Valid {
					fmt.Printf("\n%s: Required variable %s is not set\n",
						tui.ErrorStyle.Render("Error"), varStyle.Render(valResult.Field))

					if valResult.Help != "" {
						fmt.Printf("  Description: %s\n", valResult.Help)
					}

					fmt.Println("  Set this variable in your '.env' file or run `dr dotenv setup` to configure it.")
				}
			}

			os.Exit(1)
		}

		fmt.Println("\nValidation passed: all required variables are set.")
	},
}
