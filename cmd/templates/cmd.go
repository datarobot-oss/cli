// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package templates

import (
	"github.com/datarobot/cli/cmd/templates/list"
	"github.com/datarobot/cli/cmd/templates/setup"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		GroupID: "core",
		Short:   "📚 DataRobot apsplication templates commands",
		Long: `Application templates commands for ` + version.AppName + `.

Manage DataRobot AI application templates:
  • Browse available templates
  • Clone templates to your local machine
  • Set up new projects with interactive wizard

🚀 Quick start: dr templates setup`,
	}

	cmd.AddCommand(
		// clone.Cmd,  # CFX-3969 disabled for now
		list.Cmd,
		setup.Cmd,
	)

	return cmd
}
