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
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
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
			f, err := tea.LogToFile(tui.DebugLogFile, "debug")
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

		m := Model{
			initialScreen:  editorScreen,
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
			f, err := tea.LogToFile(tui.DebugLogFile, "debug")
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
