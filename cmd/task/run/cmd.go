// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package run

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

type taskRunOptions struct {
	Dir      string
	taskOpts task.RunOpts
}

func Cmd() *cobra.Command {
	var opts taskRunOptions

	cmd := &cobra.Command{
		Use:     "run [task1, task2, ...] [flags]",
		Aliases: []string{"r"},
		Short:   "Run an application template task",
		Run: func(_ *cobra.Command, args []string) {
			binaryName := "task"
			discovery := task.NewTaskDiscovery("Taskfile.gen.yaml")

			rootTaskfile, err := discovery.Discover(opts.Dir, 2)
			if err != nil {
				task.ExitWithError(err)
				return
			}

			runner := task.NewTaskRunner(task.RunnerOpts{
				BinaryName: binaryName,
				Taskfile:   rootTaskfile,
				Dir:        opts.Dir,
			})

			if !runner.Installed() {
				_, _ = fmt.Fprintln(os.Stderr, `"`+binaryName+`" binary not found in PATH. Please install Task from https://taskfile.dev/installation/`)
				os.Exit(1)

				return
			}

			taskNames := args

			if !opts.taskOpts.Silent {
				log.Printf("Running task(s): %s\n", strings.Join(taskNames, ", "))
			}

			err = runner.Run(taskNames, opts.taskOpts)
			if err != nil { //nolint: nestif
				exitCode := 1

				if exitErr, ok := err.(*exec.ExitError); ok {
					// Only propagate if --exit-code was requested
					if opts.taskOpts.ExitCode {
						if status, ok := exitErr.Sys().(interface{ ExitStatus() int }); ok {
							exitCode = status.ExitStatus()
						}
					}
				}

				_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(exitCode)
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", ".", "Directory to look for tasks.")
	cmd.Flags().BoolVarP(&opts.taskOpts.Parallel, "parallel", "p", false, "Run tasks in parallel.")
	cmd.Flags().IntVarP(&opts.taskOpts.Concurrency, "concurrency", "C", 2, "Number of concurrent tasks to run.")
	cmd.Flags().BoolVarP(&opts.taskOpts.WatchTask, "watch", "w", false, "Enables watch of the given task.")
	cmd.Flags().BoolVarP(&opts.taskOpts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")
	cmd.Flags().BoolVarP(&opts.taskOpts.ExitCode, "exit-code", "x", false, "Pass-through the exit code of the task command.")
	cmd.Flags().BoolVarP(&opts.taskOpts.Silent, "silent", "s", false, "Disables echoing.")

	return cmd
}
