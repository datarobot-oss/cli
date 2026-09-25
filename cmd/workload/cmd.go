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
	"github.com/datarobot/cli/cmd/workload/config"
	"github.com/datarobot/cli/cmd/workload/create"
	"github.com/datarobot/cli/cmd/workload/del"
	"github.com/datarobot/cli/cmd/workload/endpoint"
	"github.com/datarobot/cli/cmd/workload/get"
	"github.com/datarobot/cli/cmd/workload/list"
	"github.com/datarobot/cli/cmd/workload/logs"
	"github.com/datarobot/cli/cmd/workload/start"
	"github.com/datarobot/cli/cmd/workload/status"
	"github.com/datarobot/cli/cmd/workload/stop"
	"github.com/datarobot/cli/cmd/workload/up"
	"github.com/datarobot/cli/internal/cli"
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

	// The gate that used to hide this whole tree now hides only the two
	// commands that are not finished. Everything else is generally available;
	// `config` and `up` stay behind DATAROBOT_CLI_FEATURE_WORKLOAD=true, and
	// the adder leaves them out at registration while it is unset, so they are
	// absent from help, completion and dispatch rather than merely hidden.
	gated := func(c *cobra.Command) *cobra.Command {
		features.SetGate(c, "workload")

		return c
	}

	adder := &cli.CommandAdder{Command: cmd}
	adder.AddCommand(
		// Setup, and the one subcommand here that never calls the API: it
		// writes the committed .datarobot.yaml that `up` deploys from. Listed
		// apart from the verbs below so it does not read as one of them.
		gated(config.Cmd()),

		// The workload itself is the primary resource: direct verbs, like
		// `dr pipeline create|get|...`.
		create.Cmd(),
		del.Cmd(),
		endpoint.Cmd(),
		get.Cmd(),
		list.Cmd(),
		logs.Cmd(),
		start.Cmd(),
		status.Cmd(),
		stop.Cmd(),
		gated(up.Cmd()),
	)

	return cmd
}
