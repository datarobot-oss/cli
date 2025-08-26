// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

func SetURLAction() {
	_, err := config.GetURL(true)
	if err != nil {
		log.Fatal(err)
	}
}

var setURLCmd = &cobra.Command{
	Use:   "setURL",
	Short: "Set URL for Login to DataRobot",
	Long:  `Set URL for DataRobot to get and store that URL which can be used for other operations in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		SetURLAction() // TODO: handler errors properly
	},
}
