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

package install

import (
	"fmt"

	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

var (
	versionConstraint string
	indexURL          string
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <plugin-name>",
		Short: "Install a plugin from the remote registry",
		Long: `Install a plugin from the remote plugin registry.

The plugin name should match an entry in the plugin index.
Use --version to specify a version constraint:
  - Exact version: 1.2.3
  - Caret (compatible): ^1.2.3 (any 1.x.x >= 1.2.3)
  - Tilde (patch-level): ~1.2.3 (any 1.2.x >= 1.2.3)
  - Minimum: >=1.0.0
  - Latest: latest (default)`,
		Example: `  dr plugin install apps
  dr plugin install apps --version 1.0.0
  dr plugin install apps --version "^1.0.0"`,
		Args: cobra.ExactArgs(1),
		RunE: runInstall,
	}

	cmd.Flags().StringVar(&versionConstraint, "version", "latest", "Version constraint")
	cmd.Flags().StringVar(&indexURL, "index-url", plugin.PluginIndexURL, "URL of the plugin index")

	return cmd
}

func runInstall(_ *cobra.Command, args []string) error {
	pluginName := args[0]

	fmt.Println(tui.TitleStyle.Render("Installing Plugin"))
	fmt.Println()

	fmt.Printf("Fetching plugin index from %s...\n", indexURL)

	index, err := plugin.FetchIndex(indexURL)
	if err != nil {
		return fmt.Errorf("failed to fetch plugin index: %w", err)
	}

	pluginEntry, ok := index.Plugins[pluginName]
	if !ok {
		fmt.Println()
		fmt.Println("Available plugins:")

		for name, p := range index.Plugins {
			fmt.Printf("  - %s: %s\n", name, p.Description)
		}

		return fmt.Errorf("plugin %q not found in index", pluginName)
	}

	version, err := plugin.ResolveVersion(pluginEntry.Versions, versionConstraint)
	if err != nil {
		fmt.Println()
		fmt.Println("Available versions:")

		for _, v := range pluginEntry.Versions {
			fmt.Printf("  - %s\n", v.Version)
		}

		return fmt.Errorf("failed to resolve version: %w", err)
	}

	fmt.Printf("Installing %s version %s...\n", pluginEntry.Name, version.Version)

	if err := plugin.InstallPlugin(pluginEntry, *version); err != nil {
		return fmt.Errorf("failed to install plugin: %w", err)
	}

	fmt.Println()
	fmt.Printf(tui.SuccessStyle.Render("✓ Successfully installed %s %s"), pluginEntry.Name, version.Version)
	fmt.Println()
	fmt.Println()
	fmt.Printf("Run `dr %s --help` to get started.\n", pluginEntry.Name)

	return nil
}
