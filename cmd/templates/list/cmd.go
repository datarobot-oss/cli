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
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
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
