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
// --output-format json. isRoot distinguishes the root row from an explicit
// profiles.default row in human output only; encoding/json skips it.
type profileOutput struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	HasToken bool   `json:"has_token"`
	Active   bool   `json:"active"`

	isRoot bool
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

	// Omitted selection resolves to the root-level credentials, but the root
	// row may only claim the active marker when root actually holds
	// credentials: with no root endpoint/token nothing is authenticated, and
	// marking an empty row active would misrepresent the selection (for
	// example a config whose only profile is an explicit profiles.default).
	rootActive := active == "" && (def.Endpoint != "" || def.HasToken)

	outputs := make([]profileOutput, 0, len(profiles)+1)
	outputs = append(outputs, toProfileOutput(defaultProfileLabel, def, rootActive, true))

	for _, p := range profiles {
		outputs = append(outputs, toProfileOutput(p.Name, p, p.Name == active, false))
	}

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(os.Stdout, "profiles", outputs)
	}

	printProfilesTable(outputs)

	// Beginner guidance: with no explicit profiles at all, point at real
	// create commands. The hint is human-only -- JSON stdout must stay a
	// single machine-readable document -- and goes to stderr for the same
	// reason. An explicit profiles.default section counts as a profile, so
	// any defined section suppresses the hint.
	if len(profiles) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"No explicit profiles are defined yet. Create one with `dr auth login --profile <name>` "+
				"or `dr auth set-url --profile <name>`.")
	}

	return nil
}

func toProfileOutput(name string, info config.ProfileInfo, active, isRoot bool) profileOutput {
	return profileOutput{
		Name:     name,
		Endpoint: info.Endpoint,
		HasToken: info.HasToken,
		Active:   active,

		isRoot: isRoot,
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
		// The root row and an explicit profiles.default row would otherwise
		// render as two indistinguishable "default" rows: root gets a
		// "(root-level)" label and the explicit section "(explicit)", so a
		// reader can always tell which credentials each row stands for.
		name := o.Name
		if o.isRoot {
			name += " (root-level)"
		} else if name == defaultProfileLabel {
			name += " (explicit)"
		}

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

		t.Row(name, endpoint, token, active)
	}

	_, _ = fmt.Fprintln(os.Stdout, t.Render())
}
