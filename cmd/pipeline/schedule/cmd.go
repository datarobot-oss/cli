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

package schedule

import (
	"github.com/datarobot/cli/cmd/pipeline/schedule/create"
	"github.com/datarobot/cli/cmd/pipeline/schedule/del"
	"github.com/datarobot/cli/cmd/pipeline/schedule/get"
	"github.com/datarobot/cli/cmd/pipeline/schedule/list"
	"github.com/datarobot/cli/cmd/pipeline/schedule/update"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipelines schedule`.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage pipeline schedules",
		Long: `Manage recurring (cron) runs of locked pipeline versions.

Schedules are only valid for locked pipeline versions, so every verb
requires --pipeline and --version.`,
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
