// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package compose

import (
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

func Run() error {
	discovery := task.NewTaskDiscovery("Taskfile.yaml")

	_, err := discovery.Discover(".", 2)
	if err != nil {
		task.ExitWithError(err)
		return nil
	}

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compose",
		Short: "Compose Taskfile.yaml from multiple files in subdirectories",
		Run: func(_ *cobra.Command, _ []string) {
			err := Run()
			if err != nil {
				log.Fatal(err)
				return
			}
		},
	}
}
