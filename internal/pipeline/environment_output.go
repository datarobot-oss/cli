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

// environment_output.go centralises the human/JSON output rendering used by
// the `dr pipelines environment` verbs so each verb file stays focused on
// flag wiring.
package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

// environmentVersionJSON is the DTO for a single EnvironmentVersion in JSON output.
type environmentVersionJSON struct {
	Version     int      `json:"version"`
	Packages    []string `json:"packages"`
	Status      string   `json:"status"`
	ErrorDetail *string  `json:"error_detail,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// environmentJSON is the CLI-facing DTO for `--output-format json` of an Environment.
type environmentJSON struct {
	EnvironmentID string                   `json:"environment_id"`
	Name          string                   `json:"name"`
	Description   *string                  `json:"description,omitempty"`
	LatestVersion int                      `json:"latest_version"`
	Versions      []environmentVersionJSON `json:"versions"`
	CreatedAt     string                   `json:"created_at"`
	UpdatedAt     string                   `json:"updated_at"`
}

// environmentSummaryJSON is the CLI-facing DTO for `--output-format json` of an EnvironmentSummary.
type environmentSummaryJSON struct {
	EnvironmentID string  `json:"environment_id"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	LatestVersion int     `json:"latest_version"`
	LatestStatus  string  `json:"latest_status"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func toEnvironmentJSON(env Environment) environmentJSON {
	versions := make([]environmentVersionJSON, len(env.Versions))

	for i, v := range env.Versions {
		versions[i] = environmentVersionJSON{
			Version:     v.Version,
			Packages:    v.Packages,
			Status:      string(v.Status),
			ErrorDetail: v.ErrorDetail,
			CreatedAt:   v.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:   v.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}

	return environmentJSON{
		EnvironmentID: env.EnvironmentID,
		Name:          env.Name,
		Description:   env.Description,
		LatestVersion: env.LatestVersion,
		Versions:      versions,
		CreatedAt:     env.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     env.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toEnvironmentSummaryJSON(env EnvironmentSummary) environmentSummaryJSON {
	return environmentSummaryJSON{
		EnvironmentID: env.EnvironmentID,
		Name:          env.Name,
		Description:   env.Description,
		LatestVersion: env.LatestVersion,
		LatestStatus:  string(env.LatestStatus),
		CreatedAt:     env.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     env.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RenderEnvironment routes a single environment to JSON or human output.
func RenderEnvironment(format OutputFormat, env Environment) error {
	if format == OutputFormatJSON {
		return PrintEnvironmentJSON(env)
	}

	PrintEnvironmentHuman(env)

	return nil
}

// RenderEnvironments routes a list of environments to JSON or human output.
func RenderEnvironments(format OutputFormat, items []EnvironmentSummary) error {
	if format == OutputFormatJSON {
		return PrintEnvironmentListJSON(items)
	}

	PrintEnvironmentListHuman(items)

	return nil
}

// PrintEnvironmentJSON marshals an environment record as indented JSON through the DTO.
func PrintEnvironmentJSON(env Environment) error {
	data, err := json.MarshalIndent(toEnvironmentJSON(env), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintEnvironmentHuman renders the key facts about a single environment
// record, including its full version history.
func PrintEnvironmentHuman(env Environment) {
	desc := emptyValuePlaceholder
	if env.Description != nil && *env.Description != "" {
		desc = *env.Description
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Environment ID:\t%s\n", env.EnvironmentID)
	fmt.Fprintf(w, "Name:\t%s\n", env.Name)
	fmt.Fprintf(w, "Description:\t%s\n", desc)
	fmt.Fprintf(w, "Latest version:\tv%s\n", strconv.Itoa(env.LatestVersion))
	fmt.Fprintf(w, "Created:\t%s\n", env.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", env.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()

	if len(env.Versions) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Versions:"))

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"VERSION", "STATUS", "PACKAGES", "UPDATED"}

	updatedCol := slices.Index(headers, "UPDATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == updatedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, ver := range env.Versions {
		t.Row(
			fmt.Sprintf("v%d", ver.Version),
			string(ver.Status),
			joinPackages(ver.Packages),
			ver.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// PrintEnvironmentListJSON marshals a list of environments as indented JSON through the DTO.
func PrintEnvironmentListJSON(items []EnvironmentSummary) error {
	view := make([]environmentSummaryJSON, len(items))

	for i, env := range items {
		view[i] = toEnvironmentSummaryJSON(env)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintEnvironmentListHuman renders a lipgloss table summary of environments.
func PrintEnvironmentListHuman(items []EnvironmentSummary) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No environments found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"ENVIRONMENT ID", "NAME", "LATEST", "STATUS", "UPDATED"}

	updatedCol := slices.Index(headers, "UPDATED")

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return cellStyle.Bold(true)
			}

			if col == updatedCol {
				return dimStyle
			}

			return cellStyle
		}).
		Headers(headers...)

	for _, env := range items {
		t.Row(
			env.EnvironmentID,
			env.Name,
			fmt.Sprintf("v%d", env.LatestVersion),
			string(env.LatestStatus),
			env.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
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

// NormalizePackages takes the raw slice from a cobra StringSliceVar and
// returns a cleaned list. It returns an error when the resulting list is
// empty so callers can surface a friendly validation message.
func NormalizePackages(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))

	for _, entry := range raw {
		for _, item := range strings.Split(entry, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}

	if len(out) == 0 {
		return nil, errors.New("at least one package is required (use --package)")
	}

	return out, nil
}
