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

package enclave

import (
	"github.com/datarobot/cli/cmd/enclave/access"
	"github.com/datarobot/cli/cmd/enclave/deactivate"
	"github.com/datarobot/cli/cmd/enclave/del"
	"github.com/datarobot/cli/cmd/enclave/get"
	"github.com/datarobot/cli/cmd/enclave/list"
	"github.com/datarobot/cli/cmd/enclave/register"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "enclave",
		Aliases: []string{"enclaves", "outpost", "outposts"},
		GroupID: "core",
		Short:   "🏛️  Enclave (outpost) management commands",
		Long: `Manage enclaves — remote outpost clusters that run your DataRobot workloads.

Register a new enclave to receive its one-shot installation secrets, then
inspect, deactivate, or delete it. An enclave becomes active on its own once
the installer reports in from the cluster; there is no manual activate step.`,
	}

	features.SetGate(cmd, "enclave")

	cmd.AddCommand(
		register.Cmd(),
		get.Cmd(),
		list.Cmd(),
		deactivate.Cmd(),
		del.Cmd(),
		access.Cmd(),
	)

	return cmd
}
