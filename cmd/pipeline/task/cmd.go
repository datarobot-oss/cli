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

package task

import (
	"github.com/datarobot/cli/cmd/pipeline/task/get"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipeline task`.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Inspect pipeline tasks",
		Long: `Inspect individual tasks (@task-decorated functions) within a pipeline.

Task IDs are stable identifiers minted when a pipeline is uploaded. They
appear in the TASK ID column of ` + "`dr pipeline graph`" + ` and can be
used to fetch source code, function signature, and pipeline input values.`,
	}

	cmd.AddCommand(
		get.Cmd(),
	)

	return cmd
}
