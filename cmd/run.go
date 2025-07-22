// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

type taskRunOptions struct {
	Dir         string
	ListTasks   bool
	Concurrency int
	Parallel    bool
	WatchTask   bool
	AnswerYes   bool
	Silent      bool
	ExitCode    bool
}

func (o *taskRunOptions) RunOpts() task.RunOpts {
	return task.RunOpts{
		Concurrency: o.Concurrency,
		Parallel:    o.Parallel,
		WatchTask:   o.WatchTask,
		AnswerYes:   o.AnswerYes,
		Silent:      o.Silent,
		ExitCode:    o.ExitCode,
	}
}

func taskRunCmd() *cobra.Command { //nolint: cyclop
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
				_, _ = fmt.Fprintln(os.Stderr, "Error discovering tasks:", err)
				os.Exit(1)
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

			tasks, err := runner.ListTasks()

			if opts.ListTasks || len(args) == 0 {
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
					os.Exit(1)
					return
				}

				if !opts.Silent {
					fmt.Println("Available tasks:")
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 8, 6, ' ', 0)
				for _, t := range tasks {
					desc := strings.ReplaceAll(t.Desc, "\n", " ")

					_, _ = fmt.Fprint(w, "* ")
					_, _ = fmt.Fprint(w, t.Name)

					if len(t.Aliases) > 0 {
						_, _ = fmt.Fprintf(w, " (%s)", strings.Join(t.Aliases, ", "))
					}

					_, _ = fmt.Fprintf(w, " \t%s", desc)

					_, _ = fmt.Fprint(w, "\n")
				}

				if err = w.Flush(); err != nil {
					fmt.Fprintln(os.Stderr, "Error:", err)
					os.Exit(1)
					return
				}

				return
			}

			taskNames := args

			if !opts.Silent {
				log.Printf("Running task(s): %s\n", strings.Join(taskNames, ", "))
			}

			err = runner.Run(taskNames, opts.RunOpts())
			if err != nil { //nolint: nestif
				exitCode := 1

				if exitErr, ok := err.(*exec.ExitError); ok {
					// Only propagate if --exit-code was requested
					if opts.ExitCode {
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
	cmd.Flags().BoolVarP(&opts.ListTasks, "list", "l", false, "List all available tasks.")
	cmd.Flags().BoolVarP(&opts.Parallel, "parallel", "p", false, "Run tasks in parallel.")
	cmd.Flags().IntVarP(&opts.Concurrency, "concurrency", "C", 2, "Number of concurrent tasks to run.")
	cmd.Flags().BoolVarP(&opts.WatchTask, "watch", "w", false, "Enables watch of the given task.")
	cmd.Flags().BoolVarP(&opts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")
	cmd.Flags().BoolVarP(&opts.ExitCode, "exit-code", "x", false, "Pass-through the exit code of the task command.")
	cmd.Flags().BoolVarP(&opts.Silent, "silent", "s", false, "Disables echoing.")

	return cmd
}
