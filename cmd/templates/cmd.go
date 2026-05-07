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

package templates

import (
	"github.com/datarobot/cli/cmd/templates/list"
	"github.com/datarobot/cli/cmd/templates/setup"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "template",
		Aliases: []string{"templates"},
		GroupID: "core",
		Short:   "📚 DataRobot application templates commands",
		Long: `Application templates commands for ` + version.AppName + `.

Manage DataRobot AI application templates:
  • Browse available templates
  • Clone templates to your local machine
  • Set up new projects with interactive wizard

🚀 Quick start: dr templates setup`,
	}

	cmd.AddCommand(
		// clone.Cmd,  # CFX-3969 disabled for now
		list.Cmd,
		setup.Cmd,
	)

	return cmd
}
