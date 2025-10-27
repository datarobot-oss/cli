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
		Use:   "task",
		Short: "Run or generate Taskfile.yaml commands",
	}

	cmd.AddCommand(
		compose.Cmd(),
		list.Cmd(),
		run.Cmd(),
	)

	return cmd
}
