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

package selfplugin

import (
	"github.com/datarobot/cli/cmd/self/plugin/add"
	pluginpackage "github.com/datarobot/cli/cmd/self/plugin/package"
	"github.com/datarobot/cli/cmd/self/plugin/publish"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Plugin packaging and development tools",
	}

	cmd.AddCommand(
		add.Cmd(),
		publish.Cmd(),
		pluginpackage.Cmd(),
	)

	return cmd
}
