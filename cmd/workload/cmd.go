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

package workload

import (
	"github.com/datarobot/cli/cmd/workload/create"
	"github.com/datarobot/cli/cmd/workload/del"
	"github.com/datarobot/cli/cmd/workload/endpoint"
	"github.com/datarobot/cli/cmd/workload/env"
	"github.com/datarobot/cli/cmd/workload/get"
	"github.com/datarobot/cli/cmd/workload/list"
	"github.com/datarobot/cli/cmd/workload/logs"
	"github.com/datarobot/cli/cmd/workload/start"
	"github.com/datarobot/cli/cmd/workload/status"
	"github.com/datarobot/cli/cmd/workload/stop"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workload",
		Aliases: []string{"wl"},
		GroupID: "core",
		Short:   "🚀 Workload management commands",
		Long: `Workload management commands for your DataRobot applications.

Manage and monitor workloads in your deployment infrastructure.`,
	}

	features.SetGate(cmd, "workload")

	cmd.AddCommand(
		// The workload itself is the primary resource: direct verbs, like
		// `dr pipeline create|get|...`.
		create.Cmd(),
		del.Cmd(),
		endpoint.Cmd(),
		env.Cmd(),
		get.Cmd(),
		list.Cmd(),
		logs.Cmd(),
		start.Cmd(),
		status.Cmd(),
		stop.Cmd(),
	)

	return cmd
}
