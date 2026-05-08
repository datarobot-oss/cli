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

package code

import (
	"github.com/datarobot/cli/cmd/workload/code/checkout"
	initcmd "github.com/datarobot/cli/cmd/workload/code/init"
	"github.com/datarobot/cli/cmd/workload/code/versions"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Manage workload code synchronization.",
		Long: `Manage the local-to-remote synchronization of code for an existing
DataRobot workload artifact.

These commands maintain a '.wapi/' state directory at the project root
that tracks which artifact, catalog, and version a directory is bound
to. The model is conceptually similar to '.git/' — local work happens
in the project root, while '.wapi/' captures the remote binding and
last-synced state used to detect drift on each operation.

Subcommands:
  init       Link a directory to an existing artifact and lay down the
             '.wapi/' state. Required before any other 'code' command.
  sync       Push local edits and pull remote changes (coming soon).
  versions   List catalog versions for the linked artifact.
  checkout   Download a prior version into '.wapi/.checkouts/' for
             read-only inspection.

Artifacts must already exist before running 'init'. Create them via
'dr workload artifact create' or in the DataRobot UI — these commands
manage the *code* of an artifact, not its lifecycle.

Example:
  dr workload code init art-abc-123
  dr workload code init art-abc-123 --dir ./service`,
	}

	cmd.AddCommand(initcmd.Cmd())
	cmd.AddCommand(versions.Cmd())
	cmd.AddCommand(checkout.Cmd())

	return cmd
}
