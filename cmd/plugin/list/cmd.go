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

package list

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// PluginOutput is the JSON representation of a discovered plugin for --output-format json.
type PluginOutput struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "list",
		Short: "📋 List discovered plugins",
		Long:  "List all discovered plugins with their paths and versions. Uses cached results from CLI startup.",
		RunE:  runList,
	}

	outputformat.AddFlag(cmd, &outputFormat)

	return cmd
}

func toPluginOutputs(plugins []plugin.DiscoveredPlugin) []PluginOutput {
	outputs := make([]PluginOutput, len(plugins))
	for i, p := range plugins {
		version := p.Manifest.Version
		if version == "" {
			version = "-"
		}

		desc := p.Manifest.Description
		if desc == "" {
			desc = "-"
		}

		outputs[i] = PluginOutput{
			Name:        p.Manifest.Name,
			Version:     version,
			Description: desc,
			Path:        p.Executable,
		}
	}

	return outputs
}

func printPluginsTable(plugins []plugin.DiscoveredPlugin) error {
	fmt.Println(tui.SubTitleStyle.Render("Discovered Plugins"))

	nameStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
		Padding(0, 1)

	descStyle := tui.DimStyle.
		Padding(0, 1)

	pathStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)).
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(_, col int) lipgloss.Style {
			switch col {
			case 0:
				return nameStyle
			case 1:
				return descStyle
			default:
				return pathStyle
			}
		}).
		Headers("NAME", "VERSION", "DESCRIPTION", "PATH")

	for _, p := range plugins {
		version := p.Manifest.Version
		if version == "" {
			version = "-"
		}

		desc := p.Manifest.Description
		if desc == "" {
			desc = "-"
		}

		t.Row(p.Manifest.Name, version, desc, p.Executable)
	}

	_, _ = fmt.Fprintln(os.Stdout, t.Render())

	return nil
}

func runList(cmd *cobra.Command, _ []string) error {
	plugins, err := plugin.GetPlugins()
	if err != nil {
		return fmt.Errorf("failed to get plugins: %w", err)
	}

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		outputs := toPluginOutputs(plugins)
		return outputformat.PrintJSONEnvelope(os.Stdout, "plugins", outputs)
	}

	if len(plugins) == 0 {
		fmt.Println("No plugins discovered.")
		fmt.Println()
		fmt.Println("Plugins are discovered from:")
		fmt.Println("  1. Project-local .dr/plugins/ directory")
		fmt.Println("  2. Executables named 'dr-*' in PATH")

		return nil
	}

	return printPluginsTable(plugins)
}
