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
	"strings"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command { //nolint: cyclop
	var dir string

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List tasks",
		Run: func(_ *cobra.Command, args []string) {
			binaryName := "task"
			discovery := task.NewTaskDiscovery("Taskfile.gen.yaml")

			rootTaskfile, err := discovery.Discover(dir, 2)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error discovering tasks:", err)
				os.Exit(1)

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
				_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)

				return
			}

			fmt.Println("Available tasks:")
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
		},
	}

	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "Directory to look for tasks.")

	return cmd
}
