// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"fmt"
	"os"
	"path/filepath"
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
		// Use ParseVariablesOnly to avoid auto-populating values during manual editing
		variables := envbuilder.ParseVariablesOnly(templateLines)
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
			os.Exit(1)
		}
		dotenvFile := filepath.Join(repositoryRoot, ".env")
		templateLines, templateFileUsed := readTemplate(dotenvFile)
		variables, contents, _ := envbuilder.VariablesFromTemplate(templateLines)

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
			os.Exit(1)
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
			os.Exit(1)
		}

		_, _, _, err = writeUsingTemplateFile(dotenv)
		if err != nil {
			log.Error(err)
			os.Exit(1)
		}
	},
}

var ValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate .env and environment variable configuration against required settings",
	Run: func(_ *cobra.Command, _ []string) {
		dotenv, err := ensureInRepoWithDotenv()
		if err != nil {
			os.Exit(1)
		}

		repoRoot := filepath.Dir(dotenv)

		templateLines, _ := readTemplate(dotenv)

		// Parse variables from .env file
		parsedVars := envbuilder.ParseVariablesOnly(templateLines)

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
					fmt.Printf("\n%s: required variable %s is not set\n",
						tui.ErrorStyle.Render("Error"), varStyle.Render(valResult.Field))

					if valResult.Help != "" {
						fmt.Printf("  Description: %s\n", valResult.Help)
					}

					fmt.Println("  Set this variable in your .env file or run `dr dotenv setup` to configure it.")
				}
			}

			os.Exit(1)
		}

		fmt.Println("\nValidation passed: all required variables are set.")
	},
}
