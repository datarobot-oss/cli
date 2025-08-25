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
	"github.com/datarobot/cli/internal/base_auth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func LogoutAction() error {
	viper.Set(base_auth.DataRobotAPIKey, base_auth.DataRobotAPIKey)

	writeConfigFile()

	return nil
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from DataRobot",
	Long:  `Logout from DataRobot and clear the stored API key.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := LogoutAction()
		if err != nil {
			log.Error(err)
		}
	},
}
