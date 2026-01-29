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

package update

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

var (
	indexURL string
	checkAll bool
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [plugin-name]",
		Short: "Update a plugin to the latest version",
		Long: `Update an installed plugin to the latest available version.

If no plugin name is provided with --all, checks all installed plugins for updates.`,
		Example: `  dr plugin update apps
  dr plugin update --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: runUpdate,
	}

	cmd.Flags().StringVar(&indexURL, "index-url", plugin.PluginIndexURL, "URL of the plugin index")
	cmd.Flags().BoolVar(&checkAll, "all", false, "Update all installed plugins")

	return cmd
}

func runUpdate(_ *cobra.Command, args []string) error {
	installed, err := plugin.GetInstalledPlugins()
	if err != nil {
		return fmt.Errorf("failed to get installed plugins: %w", err)
	}

	if len(installed) == 0 {
		fmt.Println("No managed plugins installed.")

		return nil
	}

	toUpdate, err := selectPluginsToUpdate(args, installed)
	if err != nil {
		return err
	}

	// Auto-append index.json if not present
	finalIndexURL := indexURL
	if len(finalIndexURL) > 0 && finalIndexURL[len(finalIndexURL)-1] == '/' {
		finalIndexURL += "index.json"
	} else if len(finalIndexURL) > 5 && finalIndexURL[len(finalIndexURL)-5:] != ".json" {
		finalIndexURL += "/index.json"
	}

	fmt.Printf("Fetching plugin index from %s...\n", finalIndexURL)

	index, baseURL, err := plugin.FetchIndex(finalIndexURL)
	if err != nil {
		return fmt.Errorf("failed to fetch plugin index: %w", err)
	}

	fmt.Println()

	updated := updatePlugins(toUpdate, index, baseURL)

	fmt.Println()

	if updated > 0 {
		fmt.Printf("Updated %d plugin(s)\n", updated)
	} else {
		fmt.Println("All plugins are up to date.")
	}

	return nil
}

func selectPluginsToUpdate(args []string, installed []plugin.InstalledPlugin) ([]plugin.InstalledPlugin, error) {
	if len(args) > 0 {
		pluginName := args[0]

		for _, p := range installed {
			if p.Name == pluginName {
				return []plugin.InstalledPlugin{p}, nil
			}
		}

		return nil, fmt.Errorf("plugin %q is not installed as a managed plugin", pluginName)
	}

	if checkAll {
		return installed, nil
	}

	return nil, errors.New("specify a plugin name or use --all to update all plugins")
}

func updatePlugins(toUpdate []plugin.InstalledPlugin, index *plugin.PluginIndex, baseURL string) int {
	var updated int

	for _, p := range toUpdate {
		if updateSinglePlugin(p, index, baseURL) {
			updated++
		}
	}

	return updated
}

func updateSinglePlugin(p plugin.InstalledPlugin, index *plugin.PluginIndex, baseURL string) bool {
	pluginEntry, ok := index.Plugins[p.Name]
	if !ok {
		fmt.Printf("⚠ Plugin %s not found in index, skipping\n", p.Name)

		return false
	}

	latestVersion, err := plugin.ResolveVersion(pluginEntry.Versions, "latest")
	if err != nil {
		fmt.Printf("⚠ Failed to resolve latest version for %s: %v\n", p.Name, err)

		return false
	}

	if p.Version == latestVersion.Version {
		fmt.Printf("✓ %s is already at the latest version (%s)\n", p.Name, p.Version)

		return false
	}

	fmt.Printf("Updating %s from %s to %s...\n", p.Name, p.Version, latestVersion.Version)

	if err := plugin.InstallPlugin(pluginEntry, *latestVersion, baseURL); err != nil {
		fmt.Printf("✗ Failed to update %s: %v\n", p.Name, err)

		return false
	}

	successMsg := lipgloss.NewStyle().
		Foreground(tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreen)).
		Bold(true).
		Render("✓ Updated " + p.Name + " to " + latestVersion.Version)
	fmt.Println(successMsg)
	fmt.Println()

	return true
}
