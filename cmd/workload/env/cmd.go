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

package env

import (
	"github.com/datarobot/cli/cmd/workload/env/del"
	importcmd "github.com/datarobot/cli/cmd/workload/env/import"
	"github.com/datarobot/cli/cmd/workload/env/list"
	"github.com/datarobot/cli/cmd/workload/env/set"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage a workload's environment variables.",
		Long: `Manage the environment variables on the artifact a workload is running.

Environment variables live on the artifact, not the workload's runtime
settings, so changing them means finding the workload's current artifact,
editing it (in place if it is a draft, via a clone if it is locked), and
rolling the workload onto the result -- these commands do that dance for
you.

Only the workload's primary container is affected. Artifacts with
additional (sidecar) containers are not yet supported.

Subcommands:
  list     Show the current environment variables.
  set      Add or update one or more variables from NAME=VALUE arguments,
           then roll out.
  import   Add or update variables from a .env file, then roll out.
  delete   Remove one or more variables, then roll out.

Example:
  dr workload env list 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload env set 68b0c1d2e3f4a5b6c7d8e9f0 LOG_LEVEL=debug
  dr workload env import 68b0c1d2e3f4a5b6c7d8e9f0 --file production.env
  dr workload env delete 68b0c1d2e3f4a5b6c7d8e9f0 LOG_LEVEL`,
	}

	cmd.AddCommand(list.Cmd())
	cmd.AddCommand(set.Cmd())
	cmd.AddCommand(importcmd.Cmd())
	cmd.AddCommand(del.Cmd())

	return cmd
}
