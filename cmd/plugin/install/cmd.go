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

	"github.com/datarobot/cli/cmd/plugin/shared"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	versionConstraint string
	indexURL          string
	listPlugins       bool
	listVersions      bool
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
  dr plugin install apps
  dr plugin install apps --version 1.0.0
  dr plugin install apps --version "^1.0.0"
  dr plugin install apps --versions
  dr plugin install --list`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInstall,
	}

	cmd.Flags().StringVar(&versionConstraint, "version", "latest", "Version constraint")
	cmd.Flags().BoolVar(&listVersions, "versions", false, "List available versions for a plugin")
	cmd.Flags().StringVar(&indexURL, "index-url", plugin.PluginIndexURL, "URL of the plugin index")
	cmd.Flags().BoolVar(&listPlugins, "list", false, "List available plugins from the index")

	return cmd
}

func runInstall(_ *cobra.Command, args []string) error {
	finalIndexURL := shared.NormalizeIndexURL(indexURL)
	if viper.GetBool("verbose") {
		fmt.Printf("Fetching plugin index from %s...\n", finalIndexURL)
	}

	index, baseURL, err := plugin.FetchIndex(finalIndexURL)
	if err != nil {
		return fmt.Errorf("failed to fetch plugin index: %w", err)
	}

	// Handle --list flag or no args (show list by default)
	if listPlugins || len(args) == 0 {
		fmt.Println()
		fmt.Println(tui.SubTitleStyle.Render("Available Plugins"))
		printAvailablePlugins(index)

		return nil
	}

	pluginName := args[0]

	// Handle --versions flag
	if listVersions {
		pluginEntry, ok := index.Plugins[pluginName]
		if !ok {
			printAvailablePlugins(index)

			return fmt.Errorf("plugin %q not found in index", pluginName)
		}

		fmt.Println()
		fmt.Println(tui.SubTitleStyle.Render("Available Versions for " + pluginName))
		printAvailableVersions(pluginEntry.Versions)

		return nil
	}

	fmt.Println()
	fmt.Println(tui.SubTitleStyle.Render("Installing Plugin"))

	pluginEntry, ok := index.Plugins[pluginName]
	if !ok {
		printAvailablePlugins(index)

		return fmt.Errorf("plugin %q not found in index", pluginName)
	}

	version, err := plugin.ResolveVersion(pluginEntry.Versions, versionConstraint)
	if err != nil {
		printAvailableVersions(pluginEntry.Versions)

		return fmt.Errorf("failed to resolve version: %w", err)
	}

	fmt.Printf("Installing %s version %s...\n", pluginEntry.Name, version.Version)
	fmt.Printf("Downloading from: %s/%s\n", baseURL, version.URL)

	if err := plugin.InstallPlugin(pluginEntry, *version, baseURL); err != nil {
		return fmt.Errorf("failed to install plugin: %w", err)
	}

	fmt.Println()
	fmt.Println(tui.SuccessStyle.Render("✓ Successfully installed " + pluginEntry.Name + " " + version.Version))
	fmt.Println()
	fmt.Printf("Run `dr %s --help` to get started.\n", pluginEntry.Name)

	return nil
}

func printAvailablePlugins(index *plugin.PluginIndex) {
	for name, p := range index.Plugins {
		latestVersion := "-"
		if len(p.Versions) > 0 {
			latestVersion = p.Versions[0].Version
		}

		fmt.Printf("  - %s (%s): %s\n", name, latestVersion, p.Description)
	}
}

func printAvailableVersions(versions []plugin.IndexVersion) {
	for _, v := range versions {
		fmt.Printf("  - %s\n", v.Version)
	}
}
