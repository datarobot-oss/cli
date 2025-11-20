// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var dir string

	var showAll bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List tasks",
		Run: func(_ *cobra.Command, _ []string) {
			binaryName := "task"
			discovery := task.NewTaskDiscovery("Taskfile.gen.yaml")

			rootTaskfile, err := discovery.Discover(dir, 2)
			if err != nil {
				task.ExitWithError(err)
				return
			}

			runner := task.NewTaskRunner(task.RunnerOpts{
				BinaryName: binaryName,
				Taskfile:   rootTaskfile,
				Dir:        dir,
			})

			if !runner.Installed() {
				_, _ = fmt.Fprintln(os.Stderr, `"`+binaryName+`" binary not found in PATH. Please install Task from https://taskfile.dev/installation/`)

				os.Exit(1)

				return
			}

			tasks, err := runner.ListTasks()
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error: ", err)

				os.Exit(1)

				return
			}

			categories := task.GroupTasksByCategory(tasks, showAll)

			if err = task.PrintCategorizedTasks(categories, showAll); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error: ", err)

				os.Exit(1)

				return
			}
		},
	}

	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "Directory to look for tasks.")
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all tasks including less commonly used ones")

	// Register directory completion for the dir flag
	_ = cmd.RegisterFlagCompletionFunc("dir", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	})

	return cmd
}
