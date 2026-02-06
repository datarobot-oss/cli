// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/log"
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

💡 Tasks are defined in your project's 'Taskfile' and vary by template.`,
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
				_, _ = fmt.Fprintln(os.Stderr, "💡 Quick Install (choose your system):")
				_, _ = fmt.Fprintln(os.Stderr, "   🍎 macOS: brew install go-task/tap/go-task")
				_, _ = fmt.Fprintln(os.Stderr, "   🐧 Linux: sh -c \"$(curl --location https://taskfile.dev/install.sh)\"")
				_, _ = fmt.Fprintln(os.Stderr, "   🪟 Windows: choco install go-task")
				_, _ = fmt.Fprintln(os.Stderr, "")
				_, _ = fmt.Fprintln(os.Stderr, "📚 Need help? Visit: https://taskfile.dev/installation/")
				_, _ = fmt.Fprintln(os.Stderr, "")
				_, _ = fmt.Fprintln(os.Stderr, "After installing, try running your command again!")

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
					// Only propagate if '--exit-code' was requested
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

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", ".", "📁 Specify project directory (default: current directory)")
	cmd.Flags().BoolVarP(&opts.taskOpts.Parallel, "parallel", "p", false, "⚡ Run multiple tasks simultaneously for faster execution")
	cmd.Flags().IntVarP(&opts.taskOpts.Concurrency, "concurrency", "C", 2, "🔢 Number of concurrent tasks to run in parallel")
	cmd.Flags().BoolVarP(&opts.taskOpts.WatchTask, "watch", "w", false, "👀 Watch files and re-run task on changes")
	cmd.Flags().BoolVarP(&opts.taskOpts.AnswerYes, "yes", "y", false, "🚀 Skip confirmation prompts (useful for automation)")
	cmd.Flags().BoolVarP(&opts.taskOpts.ExitCode, "exit-code", "x", false, "🔄 Pass through the exact exit code from task")
	cmd.Flags().BoolVarP(&opts.taskOpts.Silent, "silent", "s", false, "🔇 Suppress task output and progress messages")

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
