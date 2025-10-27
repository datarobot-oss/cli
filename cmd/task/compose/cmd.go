// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package compose

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/task"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

func Run() error {
	discovery := task.NewTaskDiscovery("Taskfile.yaml")

	_, err := discovery.Discover(".", 2)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error discovering tasks:", err)
		os.Exit(1)

		return nil
	}

	return nil
}

func Cmd() *cobra.Command { //nolint: cyclop
	return &cobra.Command{
		Use:   "compose",
		Short: "Compose Taskfile.yaml from multiple files in subdirectories",
		Run: func(_ *cobra.Command, args []string) {
			err := Run()
			if err != nil {
				log.Fatal(err)
				return
			}
		},
	}
}
