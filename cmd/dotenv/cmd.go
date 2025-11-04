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
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

		// Default is editor screen but if we detect other Env Vars we'll potentially use wizard screen
		screen := editorScreen

		dotenvFile := filepath.Join(cwd, ".env")
		templateLines, templateFileUsed := readTemplate(dotenvFile)
		// Use parseVariablesOnly to avoid auto-populating values during manual editing
		variables := parseVariablesOnly(templateLines)
		contents := strings.Join(templateLines, "")

		if repo.IsInRepo() {
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
					variables = append(variables, variable{name: up.Env, value: up.Default})
				}
			}

			if extraEnvVarsFound {
				fmt.Println("Environment Configuration")
				fmt.Println("=========================")
				fmt.Println("")
				fmt.Println("Editing .env file with component-specific variables...")
				fmt.Println("")
				for _, fv := range userPrompts {
					fmt.Println(fv.Env + " = " + fv.Default)
				}
				fmt.Println("")
				fmt.Println("Configure required missing variables now? (y/N): ")

				reader := bufio.NewReader(os.Stdin)
				selectedOption, err := reader.ReadString('\n')
				if err != nil {
					return err
				}

				if strings.ToLower(strings.TrimSpace(selectedOption)) == "y" {
					screen = wizardScreen
				}
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

		return err
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
		dotenvFile := ".env"

		_, _, _, err := writeUsingTemplateFile(dotenvFile)
		if err != nil {
			log.Error(err)
		}
	},
}
