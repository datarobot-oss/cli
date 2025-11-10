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

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
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
	Short: "📋 List all available AI application templates",
	Long: `List all available AI application templates from DataRobot.

This command shows you all the pre-built templates you can use to quickly 
start building AI applications. Each template includes:
  • Complete application structure
  • Pre-configured components
  • Documentation and examples
  • Ready-to-deploy setup

💡 Use 'dr templates setup' for an interactive selection experience.`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		return auth.EnsureAuthenticatedE(cmd.Context())
	},
	Run: func(_ *cobra.Command, _ []string) {
		err := Run()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
