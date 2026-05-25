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

package environment

import (
	"github.com/datarobot/cli/cmd/pipeline/environment/create"
	"github.com/datarobot/cli/cmd/pipeline/environment/del"
	"github.com/datarobot/cli/cmd/pipeline/environment/list"
	"github.com/datarobot/cli/cmd/pipeline/environment/update"
	"github.com/datarobot/cli/cmd/pipeline/environment/version"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipeline environment`. It
// groups the lifecycle verbs that operate on pipeline execution
// environments (named, immutable-versioned bags of pip packages).
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "environment",
		Aliases: []string{"environments"},
		Short:   "Manage pipeline execution environments",
		Long: `Manage pipeline execution environments.

Environments are named, immutable-versioned bags of pip packages that
pipelines can be built against. Each ` + "`update`" + ` adds packages by
creating a new version; older versions can be deleted individually with
` + "`environment version delete`" + `.`,
	}

	cmd.AddCommand(
		create.Cmd(),
		list.Cmd(),
		update.Cmd(),
		del.Cmd(),
		version.Cmd(),
	)

	return cmd
}
