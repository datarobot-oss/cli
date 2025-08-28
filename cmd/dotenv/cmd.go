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

var Cmd = &cobra.Command{
	Use:   "dotenv",
	Short: "Add Datarobot credentials to .env file",
	Long:  "Generate or update .env file with Datarobot credentials",
	Run:   Run,
}

func Run(_ *cobra.Command, _ []string) {
	dotenvFile := ".env"

	_, _, _, err := writeUsingTemplateFile(dotenvFile)
	if err != nil {
		log.Error(err)
	}
}
