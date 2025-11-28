// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package task

import (
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/cmd/task/list"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "task",
		GroupID: "core",
		Short:   "🛠️ Task management commands",
		Long: `Task management commands for your DataRobot applications.

Manage and execute tasks defined in your project's 'Taskfile':
  • Run development, build, test, and deployment tasks
  • List all available tasks
  • Compose and generate task configurations

🚀 Quick start: dr run dev`,
		Run: func(cmd *cobra.Command, args []string) {
			run.Cmd().Run(cmd, args)
		},
	}

	cmd.AddCommand(
		compose.Cmd(),
		list.Cmd(),
		run.Cmd(),
	)

	return cmd
}
