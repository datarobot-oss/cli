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

package build

import (
	"github.com/datarobot/cli/cmd/workload/build/get"
	"github.com/datarobot/cli/cmd/workload/build/list"
	"github.com/datarobot/cli/cmd/workload/build/logs"
	"github.com/datarobot/cli/cmd/workload/build/trigger"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Manage workload artifact builds",
		Long: `Trigger, inspect, and read logs from container image builds for workload
artifacts.

When run inside a directory linked via 'dr workload code init', the
<artifact-id> argument may be omitted on every subcommand and is read from
.wapi/config.json.`,
	}

	cmd.AddCommand(
		get.Cmd(),
		list.Cmd(),
		logs.Cmd(),
		trigger.Cmd(),
	)

	return cmd
}
