// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dotenv",
		Short: "Commands to modify .env file",
		Long:  "Edit, generate or update .env file with Datarobot credentials",
	}

	cmd.AddCommand(
		EditCmd,
		UpdateCmd,
		WizardCmd,
	)

	return cmd
}

var EditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit .env file using built-in editor",
	Run: func(_ *cobra.Command, _ []string) {
		log.Print("Editor will be here")
	},
}

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Datarobot credentials in .env file",
	Long:  "Populate .env file with fresh Datarobot credentials",
	Run: func(_ *cobra.Command, _ []string) {
		dotenvFile := ".env"

		_, _, _, err := writeUsingTemplateFile(dotenvFile)
		if err != nil {
			log.Error(err)
		}
	},
}

var WizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Edit .env file using wizard",
	Run: func(_ *cobra.Command, _ []string) {
		log.Print("Wizard will be here")
	},
}
