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

package input

import (
	"github.com/datarobot/cli/cmd/pipeline/input/create"
	"github.com/datarobot/cli/cmd/pipeline/input/del"
	"github.com/datarobot/cli/cmd/pipeline/input/get"
	"github.com/datarobot/cli/cmd/pipeline/input/list"
	"github.com/datarobot/cli/cmd/pipeline/input/update"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipelines input`. It groups the
// CRUD verbs that operate on pipeline input sets.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "input",
		Short: "Manage pipeline input sets",
		Long: `Manage input payloads bound to a pipeline.

Inputs come in two scopes:
  - draft   : mutable; bound to the current draft of a pipeline
  - locked  : immutable; bound to a specific frozen version

When --version is supplied, the locked scope is selected automatically.
Pass --scope=draft to be explicit.`,
	}

	cmd.AddCommand(
		create.Cmd(),
		list.Cmd(),
		get.Cmd(),
		update.Cmd(),
		del.Cmd(),
	)

	return cmd
}
