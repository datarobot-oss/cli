// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

func (o *taskRunOptions) TaskfileArgs() []string {
	args := make([]string, 0, 6)

	if o.Parallel {
		args = append(args, "--parallel")
	}

	if o.WatchTask {
		args = append(args, "--watch")
	}

	if o.AnswerYes {
		args = append(args, "--yes")
	}

	if o.ExitCode {
		args = append(args, "--exit-code")
	}

	if o.Silent {
		args = append(args, "--silent")
	}

	args = append(args, "-C", strconv.Itoa(o.Concurrency))

	return args
}

func taskRunCmd() *cobra.Command {
	var opts taskRunOptions

	cmd := &cobra.Command{
		Use:     "run [task1, task2, ...] [flags]",
		Aliases: []string{"r"},
		Short:   "Run an application template task",
		Run: func(_ *cobra.Command, args []string) {
			dir := opts.Dir
			tasks, err := getAllTasks(dir)

			if opts.ListTasks || len(args) == 0 {
				if err != nil {
					fmt.Fprintln(os.Stderr, "Error:", err)
					return
				}

				fmt.Println("Available tasks:")

				for _, task := range tasks {
					fmt.Printf("  %s\t- %s\n", task.Name, task.Desc)
				}

				return
			}

			taskNames := args

			if !opts.Silent {
				fmt.Printf("Running task(s): %s\n", strings.Join(taskNames, ", "))
			}

			err = runTask(dir, taskNames, opts)
			if err != nil { //nolint: nestif
				fmt.Fprintln(os.Stderr, "Error:", err)

				if exitErr, ok := err.(*exec.ExitError); ok {
					// Only propagate if --exit-code was requested
					if opts.ExitCode {
						if status, ok := exitErr.Sys().(interface{ ExitStatus() int }); ok {
							os.Exit(status.ExitStatus())
						}
					}
				}

				os.Exit(1)
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

type Task struct {
	Name     string   `json:"name"`
	Desc     string   `json:"desc"`
	Summary  string   `json:"summary"`
	Aliases  []string `json:"aliases"`
	UpToDate bool     `json:"up_to_date"`
	Location struct {
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Taskfile string `json:"taskfile"`
	} `json:"location"`
}

type TaskList struct {
	Tasks []Task `json:"tasks"`
}

func getAllTasks(dir string) ([]Task, error) {
	// TODO: check if task is installed
	cmd := exec.Command("task", "--list", "--json")

	cmd.Dir = dir

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	var taskList TaskList

	if err := json.Unmarshal(out.Bytes(), &taskList); err != nil {
		return nil, fmt.Errorf("invalid task JSON: %w", err)
	}

	return taskList.Tasks, nil
}

func runTask(dir string, taskNames []string, opts taskRunOptions) error {
	var args []string

	args = append(args, opts.TaskfileArgs()...)
	args = append(args, taskNames...)

	cmd := exec.Command("task", args...)

	cmd.Dir = dir

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}
