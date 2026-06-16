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

package artifact

import (
	"github.com/datarobot/cli/cmd/artifact/build"
	"github.com/datarobot/cli/cmd/artifact/code"
	"github.com/datarobot/cli/cmd/artifact/create"
	"github.com/datarobot/cli/cmd/artifact/del"
	"github.com/datarobot/cli/cmd/artifact/get"
	"github.com/datarobot/cli/cmd/artifact/list"
	"github.com/datarobot/cli/cmd/artifact/lock"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "artifact",
		GroupID: "core",
		Short:   "📦 Artifact management commands",
		Long: `Artifact management commands for your DataRobot workloads.

Create and inspect artifacts, build their container images, and
synchronize local code with a remote artifact.`,
	}

	features.SetGate(cmd, "workload")

	cmd.AddCommand(
		// The artifact is the primary resource: direct verbs, like
		// `dr pipeline create|get|...`.
		create.Cmd(),
		del.Cmd(),
		get.Cmd(),
		list.Cmd(),
		lock.Cmd(),
		// Sub-resources keep their noun groups.
		build.Cmd(),
		code.Cmd(),
	)

	return cmd
}
