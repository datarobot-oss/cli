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
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// defaultProfileLabel is how the top-level (unnamed) profile is displayed
// and how it is addressed in JSON output and by `dr auth profile show`.
const defaultProfileLabel = config.DefaultProfileLabel

// profileOutput is the JSON representation of one profile for
// --output-format json.
type profileOutput struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	HasToken bool   `json:"has_token"`
	Active   bool   `json:"active"`
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "📋 List named profiles from drconfig.yaml",
		Long:  "List the default profile and every named profile stored in drconfig.yaml, marking the active one.",
		RunE:  runList,
	}

	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	def, profiles, err := config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("failed to read profiles: %w", err)
	}

	active := config.ActiveProfile()

	outputs := make([]profileOutput, 0, len(profiles)+1)
	outputs = append(outputs, toProfileOutput(defaultProfileLabel, def, active == ""))

	for _, p := range profiles {
		outputs = append(outputs, toProfileOutput(p.Name, p, p.Name == active))
	}

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(os.Stdout, "profiles", outputs)
	}

	printProfilesTable(outputs)

	return nil
}

func toProfileOutput(name string, info config.ProfileInfo, active bool) profileOutput {
	return profileOutput{
		Name:     name,
		Endpoint: info.Endpoint,
		HasToken: info.HasToken,
		Active:   active,
	}
}

func printProfilesTable(outputs []profileOutput) {
	fmt.Println(tui.SubTitleStyle.Render("DataRobot Profiles"))

	nameStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
		Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == 0 {
				return nameStyle
			}

			return dimStyle
		}).
		Headers("NAME", "ENDPOINT", "TOKEN", "ACTIVE")

	for _, o := range outputs {
		endpoint := o.Endpoint
		if endpoint == "" {
			endpoint = "-"
		}

		token := "not set"
		if o.HasToken {
			token = "set"
		}

		active := ""
		if o.Active {
			active = "✓"
		}

		t.Row(o.Name, endpoint, token, active)
	}

	_, _ = fmt.Fprintln(os.Stdout, t.Render())
}
