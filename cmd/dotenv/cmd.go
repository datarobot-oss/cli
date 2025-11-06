// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ensureInRepo checks if we're in a git repository, and returns the repo root path.
func ensureInRepo() (string, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil || repoRoot == "" {
		fmt.Println(tui.ErrorStyle.Render("Error:") + " not inside a git repository")
		fmt.Println()
		fmt.Println("Run this command from within an application template git repository.")
		fmt.Println("To create a new template, run " + tui.BaseTextStyle.Render("`dr templates setup`") + ".")

		return "", errors.New("not in git repository")
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
		fmt.Printf("%s: .env file does not exist at %s\n", tui.ErrorStyle.Render("Error"), dotenv)
		fmt.Println()
		fmt.Println("Run " + tui.BaseTextStyle.Render("`dr dotenv setup`") + " to create one.")

		return "", errors.New(".env file does not exist")
	}

	return dotenv, nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dotenv",
		GroupID: "core",
		Short:   "Commands to modify .env file",
		Long:    "Edit, generate or update .env file with Datarobot credentials",
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
	Short: "Edit .env file using built-in editor",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if viper.GetBool("debug") {
			f, err := tea.LogToFile("tea-debug.log", "debug")
			if err != nil {
				fmt.Println("fatal:", err)
				os.Exit(1)
			}
			defer f.Close()
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		dotenvFile := filepath.Join(cwd, ".env")
		templateLines, templateFileUsed := readTemplate(dotenvFile)
		// Use parseVariablesOnly to avoid auto-populating values during manual editing
		variables := parseVariablesOnly(templateLines)
		contents := strings.Join(templateLines, "")

		// Default is editor screen but if we detect other Env Vars we'll potentially use wizard screen
		screen := editorScreen
		if repo.IsInRepo() {
			if handleExtraEnvVars(variables) {
				screen = wizardScreen
			}
		}

		m := Model{
			initialScreen:  screen,
			DotenvFile:     dotenvFile,
			DotenvTemplate: templateFileUsed,
			variables:      variables,
			contents:       contents,
			SuccessCmd:     tea.Quit,
		}
		p := tea.NewProgram(
			tui.NewInterruptibleModel(m),
			tea.WithAltScreen(),
			tea.WithContext(cmd.Context()),
		)
		_, err = p.Run()

		return err
	},
}

var SetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Edit .env file using setup wizard",
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return auth.EnsureAuthenticatedE(cmd.Context())
	},
	Run: func(cmd *cobra.Command, _ []string) {
		if viper.GetBool("debug") {
			f, err := tea.LogToFile("tea-debug.log", "debug")
			if err != nil {
				fmt.Println("fatal:", err)
				os.Exit(1)
			}
			defer f.Close()
		}

		repositoryRoot, err := ensureInRepo()
		if err != nil {
			return
		}
		dotenvFile := filepath.Join(repositoryRoot, ".env")
		templateLines, templateFileUsed := readTemplate(dotenvFile)
		variables, contents, _ := variablesFromTemplate(templateLines)

		m := Model{
			initialScreen:  wizardScreen,
			DotenvFile:     dotenvFile,
			DotenvTemplate: templateFileUsed,
			variables:      variables,
			contents:       contents,
			SuccessCmd:     tea.Quit,
		}
		p := tea.NewProgram(
			tui.NewInterruptibleModel(m),
			tea.WithAltScreen(),
			tea.WithContext(cmd.Context()),
		)
		_, err = p.Run()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	},
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Automatically update Datarobot credentials in .env file",
	Long:  "Automatically populate .env file with fresh Datarobot credentials",
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return auth.EnsureAuthenticatedE(cmd.Context())
	},
	Run: func(_ *cobra.Command, _ []string) {
		dotenv, err := ensureInRepoWithDotenv()
		if err != nil {
			return
		}

		_, _, _, err = writeUsingTemplateFile(dotenv)
		if err != nil {
			log.Error(err)
		}
	},
}

var ValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate .env and environment variable configuration against required settings",
	Run: func(_ *cobra.Command, _ []string) {
		dotenv, err := ensureInRepoWithDotenv()
		if err != nil {
			return
		}

		repoRoot := filepath.Dir(dotenv)

		templateLines, _ := readTemplate(dotenv)

		variables := parseVariablesOnly(templateLines)

		userPrompts, rootSections, err := envbuilder.GatherUserPrompts(repoRoot)
		if err != nil {
			fmt.Printf("Error gathering user prompts: %v\n", err)

			return
		}

		envValues := make(map[string]string)

		for _, v := range variables {
			if v.name != "" && !v.commented {
				envValues[v.name] = v.value
			}
		}

		// Check all prompts for environment variable values, not just those in .env
		for _, prompt := range userPrompts {
			if prompt.Env != "" {
				// Environment variables override .env file values
				if existingValue, ok := os.LookupEnv(prompt.Env); ok {
					envValues[prompt.Env] = existingValue
				}
			}
		}

		requiredSections := make(map[string]bool)

		for _, root := range rootSections {
			requiredSections[root] = true
		}

		// Process prompts in order to determine which sections are enabled
		// based on selections made in .env or environment
		for _, prompt := range userPrompts {
			// Skip if this prompt's section is not required
			if !requiredSections[prompt.Section] {
				continue
			}

			// Get the value for this prompt
			envKey := prompt.Env
			if envKey == "" {
				envKey = "# " + prompt.Key
			}

			value, hasValue := envValues[envKey]

			// Check if any options with requires are selected
			if hasValue && len(prompt.Options) > 0 {
				selectedValues := strings.Split(value, ",")
				for _, option := range prompt.Options {
					if option.Requires != "" {
						// Check if this option is selected
						isSelected := false

						if option.Value != "" && slices.Contains(selectedValues, option.Value) {
							isSelected = true
						} else if option.Value == "" && slices.Contains(selectedValues, option.Name) {
							isSelected = true
						}

						if isSelected {
							requiredSections[option.Requires] = true
						}
					}
				}
			}
		}

		// Validate required variables
		hasErrors := false

		varStyle := lipgloss.NewStyle().Foreground(tui.DrPurple).Bold(true)

		for _, prompt := range userPrompts {
			// Skip if this prompt's section is not required
			if !requiredSections[prompt.Section] {
				continue
			}

			// Skip optional prompts
			if prompt.Optional {
				continue
			}

			// Get the value for this prompt
			envKey := prompt.Env
			if envKey == "" {
				envKey = "# " + prompt.Key
			}

			value, hasValue := envValues[envKey]

			if !hasValue || value == "" {
				if !hasErrors {
					fmt.Println("\nValidation errors:")
					hasErrors = true
				}

				fmt.Printf("\n%s: required variable %s is not set\n", tui.ErrorStyle.Render("Error"), varStyle.Render(envKey))

				if prompt.Help != "" {
					fmt.Printf("  Description: %s\n", prompt.Help)
				}

				fmt.Println("  Set this variable in your .env file or run `dr dotenv setup` to configure it.")
			}
		}

		// Also validate core DataRobot variables that must be present
		valueStyle := lipgloss.NewStyle().Foreground(tui.DrGreen)
		debugStyle := lipgloss.NewStyle().Foreground(tui.DrPurpleLight)

		requiredVars := []string{"DATAROBOT_ENDPOINT", "DATAROBOT_API_TOKEN"}

		fmt.Println(debugStyle.Render("\nValidating required variables:"))

		for _, requiredVar := range requiredVars {
			value := ""

			// Check if it exists in .env file (and is not commented)
			for _, v := range variables {
				if v.name == requiredVar && !v.commented {
					value = v.value
					fmt.Printf("  %s: found in .env with value %s\n",
						varStyle.Render(requiredVar), valueStyle.Render(value))
					break
				}
			}

			// Check environment variable (overrides .env file)
			if envValue, ok := os.LookupEnv(requiredVar); ok {
				value = envValue
				fmt.Printf("  %s: found in environment with value %s\n",
					varStyle.Render(requiredVar), valueStyle.Render(envValue))
			}

			if value == "" {
				if !hasErrors {
					fmt.Println("\nValidation errors:")
					hasErrors = true
				}

				fmt.Printf("\n%s: required variable %s is not set\n", tui.ErrorStyle.Render("Error"), varStyle.Render(requiredVar))
				fmt.Println("  Set this variable in your .env file or run `dr dotenv setup` to configure it.")
			}
		}

		if hasErrors {
			os.Exit(1)
		}

		fmt.Println("\nValidation passed: all required variables are set.")
	},
}
