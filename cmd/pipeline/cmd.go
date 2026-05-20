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

package pipeline

import (
	"github.com/datarobot/cli/cmd/pipeline/create"
	"github.com/datarobot/cli/cmd/pipeline/del"
	"github.com/datarobot/cli/cmd/pipeline/get"
	"github.com/datarobot/cli/cmd/pipeline/graph"
	"github.com/datarobot/cli/cmd/pipeline/list"
	"github.com/datarobot/cli/cmd/pipeline/lock"
	"github.com/datarobot/cli/cmd/pipeline/run"
	"github.com/datarobot/cli/cmd/pipeline/update"
	"github.com/datarobot/cli/cmd/pipeline/version"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipeline",
		Aliases: []string{"pipelines"},
		GroupID: "core",
		Short:   "Pipelines API management commands",
		Long: `Manage AI/ML pipelines orchestrated by Covalent.

Create, list, inspect, and update pipelines registered with the
DataRobot pipelines service. Sub-commands are also available for managing
input payloads, runs, and recurring schedules.`,
	}

	features.SetGate(cmd, "pipeline")

	cmd.AddCommand(
		create.Cmd(),
		get.Cmd(),
		list.Cmd(),
		update.Cmd(),
		del.Cmd(),
		lock.Cmd(),
		version.Cmd(),
		graph.Cmd(),
		run.Cmd(),
	)

	return cmd
}
