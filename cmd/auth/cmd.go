// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		GroupID: "core",
		Short:   "🔐 DataRobot authentication commands",
		Long: `Authentication commands for ` + version.AppName + `.

Manage your DataRobot credentials and connection settings:
  • Configure your DataRobot environment URL
  • Login using OAuth authentication
  • Logout and clear stored credentials

🚀 Quick start: dr auth set-url && dr auth login`,
	}

	cmd.AddCommand(
		checkCmd,
		loginCmd,
		logoutCmd,
		setURLCmd,
	)

	return cmd
}
