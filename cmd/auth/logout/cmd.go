// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package logout

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Run(_ *cobra.Command, _ []string) {
	viper.Set(config.DataRobotAPIKey, "")

	auth.WriteConfigFile()
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out from DataRobot.",
		Long:  `Log out from DataRobot and clear the stored API key.`,
		Run:   Run,
	}
}
