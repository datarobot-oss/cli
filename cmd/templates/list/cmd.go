// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/spf13/cobra"
)

func Run() error {
	templateList, err := drapi.GetTemplates()
	if err != nil {
		return err
	}

	for _, template := range templateList.Templates {
		fmt.Printf("ID: %s\tName: %s\n", template.ID, template.Name)
	}

	return nil
}

var Cmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := Run()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}

func RunTea() error {
	templateList, _ := drapi.GetTemplates()
	m := NewModel(templateList.Templates)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	return err
}

var TeaCmd = &cobra.Command{
	Use:   "list_tea",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := RunTea()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
