// Copyright 2026 DataRobot, Inc. and its affiliates.
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
	"github.com/datarobot/cli/cmd/pipeline/run/task/get"
	"github.com/datarobot/cli/cmd/pipeline/run/task/list"
	"github.com/datarobot/cli/cmd/pipeline/run/task/logs"
	"github.com/datarobot/cli/cmd/pipeline/run/task/result"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipeline run task`.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Inspect individual task executions within a run",
		Long: `Inspect the per-@task execution records within a pipeline run.

Each @task function in a pipeline becomes an electron; these subcommands
expose the lifecycle, logs, and return value of individual electrons.`,
	}

	cmd.AddCommand(
		list.Cmd(),
		get.Cmd(),
		logs.Cmd(),
		result.Cmd(),
	)

	return cmd
}
