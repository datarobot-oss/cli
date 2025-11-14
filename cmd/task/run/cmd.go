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
	"path/filepath"
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
		Short:   "🚀 Run application tasks",
		Long: `Run tasks defined in your application template.

Common tasks include:
  🏃 dev              Start development server
  🔨 build            Build production version
  🧪 test             Run all tests
  🚀 deploy           Deploy to DataRobot
  🔍 lint             Check code quality

Examples:
  dr run dev                    # Start development server
  dr run build deploy           # Build and deploy
  dr run test --parallel        # Run tests in parallel
  dr run --list                 # Show all available tasks

💡 Tasks are defined in your project's Taskfile and vary by template.`,
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
				_, _ = fmt.Fprintln(os.Stderr, "❌ Task runner not found")
				_, _ = fmt.Fprintln(os.Stderr, "")
				_, _ = fmt.Fprintln(os.Stderr, "The 'task' binary is required to run application tasks.")
				_, _ = fmt.Fprintln(os.Stderr, "")
				_, _ = fmt.Fprintln(os.Stderr, "🛠️  Install Task:")
				_, _ = fmt.Fprintln(os.Stderr, "   • macOS: brew install go-task/tap/go-task")
				_, _ = fmt.Fprintln(os.Stderr, "   • Linux: sh -c \"$(curl --location https://taskfile.dev/install.sh)\"")
				_, _ = fmt.Fprintln(os.Stderr, "   • Windows: choco install go-task")
				_, _ = fmt.Fprintln(os.Stderr, "")
				_, _ = fmt.Fprintln(os.Stderr, "📚 More info: https://taskfile.dev/installation/")

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
				} else {
					// Only print error if it's not an exit error (task already showed its error)
					_, _ = fmt.Fprintln(os.Stderr, "Error: ", err)
				}

				os.Exit(exitCode)
			}
		},
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return completeTaskNames(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", ".", "Directory to look for tasks.")
	cmd.Flags().BoolVarP(&opts.taskOpts.Parallel, "parallel", "p", false, "Run tasks in parallel.")
	cmd.Flags().IntVarP(&opts.taskOpts.Concurrency, "concurrency", "C", 2, "Number of concurrent tasks to run.")
	cmd.Flags().BoolVarP(&opts.taskOpts.WatchTask, "watch", "w", false, "Enables watch of the given task.")
	cmd.Flags().BoolVarP(&opts.taskOpts.AnswerYes, "yes", "y", false, "Assume \"yes\" as answer to all prompts.")
	cmd.Flags().BoolVarP(&opts.taskOpts.ExitCode, "exit-code", "x", false, "Pass-through the exit code of the task command.")
	cmd.Flags().BoolVarP(&opts.taskOpts.Silent, "silent", "s", false, "Disables echoing.")

	// Register directory completion for the dir flag
	_ = cmd.RegisterFlagCompletionFunc("dir", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	})

	return cmd
}

// completeTaskNames provides shell completion for task names
func completeTaskNames(opts *taskRunOptions) ([]string, cobra.ShellCompDirective) {
	binaryName := "task"

	// Try to find a Taskfile - check for standard Taskfile first,
	// then fall back to generated template Taskfile
	var taskfilePath string

	// Check for standard Taskfile.yaml (used in CLI repo itself)
	standardTaskfile := filepath.Join(opts.Dir, "Taskfile.yaml")
	if _, err := os.Stat(standardTaskfile); err == nil {
		taskfilePath = standardTaskfile
	} else {
		// Try template discovery with Taskfile.gen.yaml
		discovery := task.NewTaskDiscovery("Taskfile.gen.yaml")

		discoveredTaskfile, err := discovery.Discover(opts.Dir, 2)
		if err != nil {
			// No Taskfile found - return no completions
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		taskfilePath = discoveredTaskfile
	}

	runner := task.NewTaskRunner(task.RunnerOpts{
		BinaryName: binaryName,
		Taskfile:   taskfilePath,
		Dir:        opts.Dir,
	})

	if !runner.Installed() {
		return nil, cobra.ShellCompDirectiveError
	}

	tasks, err := runner.ListTasks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Build completion suggestions with task name and description
	completions := make([]string, 0, len(tasks))

	for _, t := range tasks {
		desc := t.Desc
		if desc == "" {
			desc = t.Summary
		}
		// Format: "taskname\tdescription"
		completions = append(completions, fmt.Sprintf("%s\t%s", t.Name, desc))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
