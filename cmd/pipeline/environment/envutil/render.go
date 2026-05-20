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

// render.go centralises the human/JSON output rendering used by the
// `dr pipelines environment` verbs so each verb file stays focused on
// flag wiring.

package envutil

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// PrintEnvironmentJSON marshals an environment record as indented JSON.
func PrintEnvironmentJSON(env pipeline.Environment) error {
	return printJSON(env)
}

// PrintEnvironmentHuman renders the key facts about a single environment
// record, including its full version history.
func PrintEnvironmentHuman(env pipeline.Environment) {
	desc := "\u2014"
	if env.Description != nil && *env.Description != "" {
		desc = *env.Description
	}

	fmt.Println(tui.BaseTextStyle.Render("Environment ID: " + env.EnvironmentID))
	fmt.Println(tui.BaseTextStyle.Render("Name:           " + env.Name))
	fmt.Println(tui.BaseTextStyle.Render("Description:    " + desc))
	fmt.Println(tui.BaseTextStyle.Render("Latest version: v" + strconv.Itoa(env.LatestVersion)))
	fmt.Println(tui.DimStyle.Render("Created:        " + env.CreatedAt))
	fmt.Println(tui.DimStyle.Render("Updated:        " + env.UpdatedAt))

	if len(env.Versions) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Versions:"))

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "VERSION\tSTATUS\tPACKAGES\tUPDATED")

	for _, ver := range env.Versions {
		fmt.Fprintf(writer, "v%d\t%s\t%s\t%s\n",
			ver.Version, ver.Status, joinPackages(ver.Packages), ver.UpdatedAt,
		)
	}

	_ = writer.Flush()
}

// PrintEnvironmentListJSON marshals a list of environments as indented JSON.
func PrintEnvironmentListJSON(items []pipeline.EnvironmentSummary) error {
	return printJSON(items)
}

// PrintEnvironmentListHuman renders a tabular summary of environments.
func PrintEnvironmentListHuman(items []pipeline.EnvironmentSummary) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No environments found"))

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "ENVIRONMENT_ID\tNAME\tLATEST\tSTATUS\tUPDATED")

	for _, env := range items {
		fmt.Fprintf(writer, "%s\t%s\tv%d\t%s\t%s\n",
			env.EnvironmentID, env.Name, env.LatestVersion, env.LatestStatus, env.UpdatedAt,
		)
	}

	_ = writer.Flush()
}

// joinPackages collapses a package slice into a single comma-separated
// string for tabular display, truncating at a reasonable width so the
// table stays readable in a typical terminal.
func joinPackages(packages []string) string {
	const maxLen = 60

	joined := strings.Join(packages, ",")
	if len(joined) <= maxLen {
		return joined
	}

	return joined[:maxLen-3] + "..."
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}
