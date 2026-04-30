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
	"github.com/datarobot/cli/cmd/workload/artifact/create"
	"github.com/datarobot/cli/cmd/workload/artifact/get"
	"github.com/datarobot/cli/cmd/workload/artifact/list"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage workload artifacts",
		Long:  `Manage workload artifacts in your DataRobot deployment infrastructure.`,
	}

	cmd.AddCommand(
		create.Cmd(),
		get.Cmd(),
		list.Cmd(),
	)

	return cmd
}
