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

package run

import (
	"github.com/datarobot/cli/cmd/pipeline/run/cancel"
	"github.com/datarobot/cli/cmd/pipeline/run/create"
	"github.com/datarobot/cli/cmd/pipeline/run/get"
	"github.com/datarobot/cli/cmd/pipeline/run/list"
	"github.com/datarobot/cli/cmd/pipeline/run/status"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipelines run`.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Manage pipeline runs",
		Long: `Trigger and inspect runs (single executions) of a pipeline.

Runs come in two scopes:
  - draft   : executes against the in-flight draft of a pipeline
  - locked  : executes against a specific frozen version

When --version is supplied, the locked scope is selected automatically.`,
	}

	cmd.AddCommand(
		create.Cmd(),
		list.Cmd(),
		get.Cmd(),
		status.Cmd(),
		cancel.Cmd(),
	)

	return cmd
}
