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

package job

import (
	"github.com/datarobot/cli/cmd/job/cancel"
	"github.com/datarobot/cli/cmd/job/create"
	"github.com/datarobot/cli/cmd/job/del"
	"github.com/datarobot/cli/cmd/job/get"
	"github.com/datarobot/cli/cmd/job/list"
	"github.com/datarobot/cli/cmd/job/logs"
	"github.com/datarobot/cli/cmd/job/run"
	"github.com/datarobot/cli/cmd/job/runs"
	"github.com/datarobot/cli/cmd/job/update"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "job",
		Aliases: []string{"jobs"},
		GroupID: "core",
		Short:   "🏁 Run-to-completion job commands",
		Long: `Manage run-to-completion jobs: code that runs once and ends, such as training or batch scoring.

A job stores the job artifact its runs execute and the runtime they start with
(resource bundle, Enclave placement). Each run is started on demand, reports
how it ended (exit code and reason) and keeps its logs. The job's code is an
ordinary job artifact, built with the artifact commands.

Needs the Workload API's jobs switch on the cluster. Hidden until
DATAROBOT_CLI_FEATURE_JOB=true.`,
	}

	features.SetGate(cmd, "job")

	cmd.AddCommand(
		create.Cmd(),
		list.Cmd(),
		get.Cmd(),
		update.Cmd(),
		del.Cmd(),
		run.Cmd(),
		runs.Cmd(),
		cancel.Cmd(),
		logs.Cmd(),
	)

	return cmd
}
