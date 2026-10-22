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
	"github.com/datarobot/cli/cmd/build/studio"
	"github.com/datarobot/cli/internal/features"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "build",
		GroupID: "core",
		Short:   "🧱 Build DataRobot applications",
		Long: `Build commands for DataRobot applications.

Open Build Studio to design and build agents with AI assistance:
  dr build studio`,
	}

	features.SetGate(cmd, "build-studio")

	cmd.AddCommand(
		studio.Cmd(),
	)

	return cmd
}
