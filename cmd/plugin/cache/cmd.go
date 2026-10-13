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

// Package cache provides commands to inspect and manage the plugin
// discovery cache used to speed up CLI startup (CFX-4726).
package cache

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	internalPlugin "github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// CacheStatusOutput is the JSON representation of the cache state for
// --output-format json.
type CacheStatusOutput struct {
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	SizeBytes     int64  `json:"size_bytes"`
	Entries       int    `json:"entries"`
	OkEntries     int    `json:"ok_entries"`
	FailedEntries int    `json:"failed_entries"`
	OldestFetch   string `json:"oldest_fetch,omitempty"`
	NewestFetch   string `json:"newest_fetch,omitempty"`
	TTL           string `json:"ttl"`
	TTLSource     string `json:"ttl_source"`
}

// Cmd returns the `dr plugin cache` command group.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "🗂 Manage the plugin discovery cache",
		Long:  "Inspect and manage the plugin manifest discovery cache used to speed up CLI startup.",
	}

	cmd.AddCommand(
		clearCmd(),
		statusCmd(),
	)

	return cmd
}

func clearCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "clear",
		Short:   "🧹 Delete the plugin discovery cache",
		Long:    "Delete the cached plugin manifest discovery results. The next CLI invocation re-probes all plugins from scratch.",
		Example: "  dr plugin cache clear",
		Args:    cobra.NoArgs,
		RunE:    runClear,
	}
}

func statusCmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "📋 Show plugin discovery cache status",
		Long:    "Show the plugin discovery cache location, entry counts, and the effective TTL with its source.",
		Example: "  dr plugin cache status\n  dr plugin cache status --output-format json",
		Args:    cobra.NoArgs,
		RunE:    runStatus,
	}

	outputformat.AddFlag(cmd, &outputFormat)

	return cmd
}

// runClear deletes the cache file. Clearing a cache that does not exist is a
// success: the caller's goal (no cache) is already satisfied.
func runClear(cmd *cobra.Command, _ []string) error {
	path, err := internalPlugin.DiscoveryCachePath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plugin discovery cache: %w", err)
	}

	fmt.Println(tui.SuccessStyle.Render("✓ Plugin discovery cache cleared"))

	return nil
}

// runStatus reports the cache state. Read-only: it never creates the cache
// file, and its TTL display matches what startup discovery actually used
// (flag first, then env, then config/default).
func runStatus(cmd *cobra.Command, _ []string) error {
	path, err := internalPlugin.DiscoveryCachePath()
	if err != nil {
		return err
	}

	output := buildStatusOutput(cmd, path)

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(os.Stdout, "cache", output)
	}

	return printStatusTable(output)
}

// buildStatusOutput collects the cache state for both output formats.
func buildStatusOutput(cmd *cobra.Command, path string) CacheStatusOutput {
	stats := internalPlugin.InspectCache(path)

	output := CacheStatusOutput{
		Path:          path,
		Exists:        stats.Exists,
		SizeBytes:     stats.SizeBytes,
		Entries:       stats.OkEntries + stats.FailedEntries,
		OkEntries:     stats.OkEntries,
		FailedEntries: stats.FailedEntries,
		TTL:           internalPlugin.DiscoveryCacheTTL().String(),
		TTLSource:     ttlSource(cmd),
	}

	if !stats.OldestFetch.IsZero() {
		output.OldestFetch = stats.OldestFetch.UTC().Format(rfc3339UTC)
	}

	if !stats.NewestFetch.IsZero() {
		output.NewestFetch = stats.NewestFetch.UTC().Format(rfc3339UTC)
	}

	return output
}

// rfc3339UTC is the timestamp format used for fetch times in both outputs.
const rfc3339UTC = "2006-01-02T15:04:05Z07:00"

// ttlSource names where the effective TTL came from, in the same precedence
// order startup discovery uses.
func ttlSource(cmd *cobra.Command) string {
	if flag := cmd.Root().PersistentFlags().Lookup(internalPlugin.DiscoveryCacheKey); flag != nil && flag.Changed {
		return "flag"
	}

	if os.Getenv(internalPlugin.DiscoveryCacheEnvVar) != "" {
		return "env"
	}

	if viperx.IsSet(internalPlugin.DiscoveryCacheKey) {
		return "config"
	}

	return "default"
}

// printStatusTable renders the cache state as a two-column table.
func printStatusTable(output CacheStatusOutput) error {
	fmt.Println(tui.SubTitleStyle.Render("Plugin Discovery Cache"))

	labelStyle := tui.BaseTextStyle.
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
		Padding(0, 1)

	valueStyle := tui.DimStyle.
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == 0 {
				return labelStyle
			}

			return valueStyle
		}).
		Headers("PROPERTY", "VALUE")

	exists := "no"
	if output.Exists {
		exists = "yes"
	}

	oldest := output.OldestFetch
	if oldest == "" {
		oldest = "-"
	}

	newest := output.NewestFetch
	if newest == "" {
		newest = "-"
	}

	t.Row("Path", output.Path)
	t.Row("Exists", exists)
	t.Row("Size", fmt.Sprintf("%d bytes", output.SizeBytes))
	t.Row("Entries", fmt.Sprintf("%d (%d ok, %d failed)", output.Entries, output.OkEntries, output.FailedEntries))
	t.Row("Oldest fetch", oldest)
	t.Row("Newest fetch", newest)
	t.Row("TTL", fmt.Sprintf("%s (%s)", output.TTL, output.TTLSource))

	_, _ = fmt.Fprintln(os.Stdout, t.Render())

	return nil
}
