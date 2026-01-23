// Copyright 2025 DataRobot, Inc. and its affiliates.
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
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered plugins",
		Long:  "List all discovered plugins with their paths and versions. Uses cached results from CLI startup.",
		RunE:  runList,
	}
}

func runList(_ *cobra.Command, _ []string) error {
	plugins, err := plugin.GetPlugins()
	if err != nil {
		return fmt.Errorf("failed to get plugins: %w", err)
	}

	if len(plugins) == 0 {
		fmt.Println("No plugins discovered.")
		fmt.Println()
		fmt.Println("Plugins are discovered from:")
		fmt.Println("  1. Project-local .dr/plugins/ directory")
		fmt.Println("  2. Executables named 'dr-*' in PATH")

		return nil
	}

	titleColor := tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark)
	nameColor := tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)
	descColor := tui.GetAdaptiveColor(tui.DrGray, tui.DrGrayDark)
	pathColor := tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)
	borderColor := tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		MarginBottom(1)

	fmt.Println(titleStyle.Render("Discovered Plugins"))
	fmt.Println()

	nameStyle := lipgloss.NewStyle().
		Foreground(nameColor).
		Padding(0, 1)

	descStyle := lipgloss.NewStyle().
		Foreground(descColor).
		Padding(0, 1)

	pathStyle := lipgloss.NewStyle().
		Foreground(pathColor).
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(borderColor)).
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
