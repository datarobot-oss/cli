// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import "github.com/spf13/cobra"

type taskRunOptions struct {
	ListTasks bool
}

func taskRunCmd() *cobra.Command {
	var opts taskRunOptions

	var cmd = &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Short:   "Run an application template task",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: implement the run command
		},
	}

	cmd.Flags().BoolP("list", "l", opts.ListTasks, "List all available tasks")

	return cmd
}
