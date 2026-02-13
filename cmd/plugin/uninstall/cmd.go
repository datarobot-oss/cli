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

package uninstall

import (
	"fmt"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall <plugin-name>",
		Short:   "Uninstall a managed plugin",
		Long:    "Remove a plugin that was installed via `dr plugin install`.",
		Example: "  dr plugin uninstall assist",
		Args:    cobra.ExactArgs(1),
		RunE:    runUninstall,
	}
}

func runUninstall(_ *cobra.Command, args []string) error {
	pluginName := args[0]

	installed, err := plugin.GetInstalledPlugins()
	if err != nil {
		return fmt.Errorf("failed to get installed plugins: %w", err)
	}

	var found bool

	for _, p := range installed {
		if p.Name == pluginName {
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("plugin %q is not installed as a managed plugin", pluginName)
	}

	fmt.Printf("Uninstalling %s...\n", pluginName)

	if err := plugin.UninstallPlugin(pluginName); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(tui.SuccessStyle.Render("✓ Successfully uninstalled " + pluginName))
	fmt.Println()

	return nil
}
