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

package task

import (
	"github.com/datarobot/cli/cmd/task/compose"
	"github.com/datarobot/cli/cmd/task/list"
	"github.com/datarobot/cli/cmd/task/run"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "task",
		GroupID: "core",
		Short:   "🛠️ Task management commands",
		Long: `Task management commands for your DataRobot applications.

Manage and execute tasks defined in your project's 'Taskfile':
  • Run development, build, test, and deployment tasks
  • List all available tasks
  • Compose and generate task configurations

🚀 Quick start: dr run dev`,
		Run: func(cmd *cobra.Command, args []string) {
			run.Cmd().Run(cmd, args)
		},
	}

	cmd.AddCommand(
		compose.Cmd(),
		list.Cmd(),
		run.Cmd(),
	)

	return cmd
}
