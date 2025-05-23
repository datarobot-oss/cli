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
	"strings"

	"github.com/spf13/cobra"
)

type taskRunOptions struct {
	Dir           string
	ListTasks     bool
	RunInParallel bool
}

func taskRunCmd() *cobra.Command {
	var opts taskRunOptions

	cmd := &cobra.Command{
		Use:     "run [TASK1, TASK2]",
		Aliases: []string{"r"},
		Short:   "Run an application template task",
		Run: func(_ *cobra.Command, args []string) {
			dir := opts.Dir
			tasks, err := getAllTasks(dir)

			if opts.ListTasks || len(args) == 0 {
				if err != nil {
					fmt.Println("Error:", err)
					return
				}

				fmt.Println("Available tasks:")

				for _, task := range tasks {
					fmt.Printf("  %s\t- %s\n", task.Name, task.Desc)
				}

				return
			}

			taskNames := args

			//if _, exists := tasks[taskName]; !exists {
			//	return suggestTasks(taskName, tasks)
			//}

			err = runTask(dir, taskNames, opts.RunInParallel)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", ".", "Directory to look for tasks")
	cmd.Flags().BoolVarP(&opts.ListTasks, "list", "l", false, "List all available tasks")
	cmd.Flags().BoolVarP(&opts.RunInParallel, "parallel", "p", false, "Run tasks in parallel")

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

func runTask(dir string, taskNames []string, parallel bool) error {
	args := taskNames

	if parallel {
		args = append([]string{"--parallel"}, taskNames...)
	}

	cmd := exec.Command("task", args...)

	cmd.Dir = dir

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Printf("Running task(s): %s\n", strings.Join(taskNames, ", "))

	return cmd.Run()
}
