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

package workload

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

const (
	timestampFormat       = "2006-01-02 15:04 UTC"
	emptyValuePlaceholder = "—"
)

func RenderArtifact(format outputformat.OutputFormat, artifact Artifact) error {
	if format == outputformat.OutputFormatJSON {
		return printArtifactJSON(artifact)
	}

	printArtifactDetails(artifact)

	return nil
}

func RenderArtifacts(format outputformat.OutputFormat, artifacts []Artifact) error {
	if format == outputformat.OutputFormatJSON {
		return printArtifactsJSON(artifacts)
	}

	printArtifactsTable(artifacts)

	return nil
}

func printArtifactJSON(artifact Artifact) error {
	data, err := json.MarshalIndent(NewArtifactOutput(artifact), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printArtifactsJSON(artifacts []Artifact) error {
	outputs := make([]ArtifactOutput, 0, len(artifacts))

	for _, a := range artifacts {
		outputs = append(outputs, NewArtifactOutput(a))
	}

	return outputformat.PrintJSONEnvelope(os.Stdout, "artifacts", outputs)
}

func printArtifactDetails(artifact Artifact) {
	catalogID, versionID := emptyValuePlaceholder, emptyValuePlaceholder

	if codeRef := ExtractCodeRef(artifact); codeRef != nil {
		catalogID = codeRef.CatalogID
		versionID = codeRef.CatalogVersionID
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", artifact.ID)
	fmt.Fprintf(w, "Name:\t%s\n", artifact.Name)
	fmt.Fprintf(w, "Status:\t%s\n", artifact.Status)
	fmt.Fprintf(w, "Catalog ID:\t%s\n", catalogID)
	fmt.Fprintf(w, "Version ID:\t%s\n", versionID)
	fmt.Fprintf(w, "Created:\t%s\n", artifact.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", artifact.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

// Build log levels in ascending severity. Used by FilterLogsByLevel to drop
// records below the requested threshold; ordering mirrors the python logging
// numeric levels but compared by name (the server emits "DEBUG", "INFO",
// "WARNING"/"WARN", "ERROR", "CRITICAL"). Unknown levels are passed through.
var buildLogLevelRank = map[string]int{
	"DEBUG":    10,
	"INFO":     20,
	"WARN":     30,
	"WARNING":  30,
	"ERROR":    40,
	"CRITICAL": 50,
}

// FilterLogsByLevel returns the subset of entries whose Levelname is at or
// above the given minimum. The threshold is matched case-insensitively;
// "info" drops DEBUG, "debug" keeps everything, an unknown threshold passes
// the input through unchanged so users can opt out of filtering with a
// nonsense value if desired.
func FilterLogsByLevel(entries []BuildLogEntry, minLevel string) []BuildLogEntry {
	threshold, ok := buildLogLevelRank[strings.ToUpper(minLevel)]
	if !ok {
		return entries
	}

	out := make([]BuildLogEntry, 0, len(entries))

	for _, e := range entries {
		rank, known := buildLogLevelRank[strings.ToUpper(e.Levelname)]
		if !known {
			out = append(out, e)

			continue
		}

		if rank >= threshold {
			out = append(out, e)
		}
	}

	return out
}

func RenderBuild(format outputformat.OutputFormat, build Build) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(NewBuildOutput(build))
	}

	printBuildDetails(build)

	return nil
}

func RenderBuilds(format outputformat.OutputFormat, builds []Build) error {
	if format == outputformat.OutputFormatJSON {
		outputs := make([]BuildOutput, 0, len(builds))

		for _, b := range builds {
			outputs = append(outputs, NewBuildOutput(b))
		}

		return printJSON(outputs)
	}

	printBuildsTable(builds)

	return nil
}

func RenderBuildTrigger(format outputformat.OutputFormat, resp BuildTriggerResponse) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(resp)
	}

	for _, id := range resp.BuildIDs {
		fmt.Println(id)
	}

	return nil
}

// RenderBuildSummaries emits a list of summaries. Text mode walks each one
// through RenderBuildSummary; JSON mode emits the whole slice as one array
// document so `--wait` scripts always get a single `jq`-able value even
// when multiple build IDs were triggered.
func RenderBuildSummaries(format outputformat.OutputFormat, summaries []BuildSummary) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(summaries)
	}

	for _, s := range summaries {
		if err := RenderBuildSummary(format, s); err != nil {
			return err
		}
	}

	return nil
}

// RenderBuildSummary renders the terminal-state summary for `--wait`.
// In text mode the human one-liner goes to stdout; the failure log tail
// (when present) goes to stderr so stdout stays clean for script callers
// reading both build ID and summary line. JSON mode emits one document on
// stdout including the LogTail in-document.
func RenderBuildSummary(format outputformat.OutputFormat, summary BuildSummary) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(summary)
	}

	dur := fmt.Sprintf("%ds", summary.DurationSeconds)

	if summary.ImageURI != "" {
		fmt.Printf("Build %s: %s in %s (image: %s)\n", summary.BuildID, summary.Status, dur, summary.ImageURI)
	} else {
		fmt.Printf("Build %s: %s in %s\n", summary.BuildID, summary.Status, dur)
	}

	if len(summary.LogTail) > 0 {
		fmt.Fprintf(os.Stderr, "--- last %d log lines ---\n", len(summary.LogTail))

		for _, entry := range summary.LogTail {
			fmt.Fprintln(os.Stderr, formatLogLine(entry))
		}
	}

	return nil
}

func RenderBuildLogs(format outputformat.OutputFormat, entries []BuildLogEntry) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(entries)
	}

	for _, entry := range entries {
		fmt.Println(formatLogLine(entry))
	}

	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printBuildDetails(build Build) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", build.ID)

	if build.Name != "" {
		fmt.Fprintf(w, "Name:\t%s\n", build.Name)
	}

	fmt.Fprintf(w, "Artifact ID:\t%s\n", build.ArtifactID)
	fmt.Fprintf(w, "Status:\t%s\n", build.Status)
	fmt.Fprintf(w, "Created:\t%s\n", build.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", build.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

func printBuildsTable(builds []Build) {
	if len(builds) == 0 {
		fmt.Println("No builds found.")

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)
	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"BUILD ID", "NAME", "ARTIFACT ID", "STATUS", "CREATED", "UPDATED"}
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

	for _, b := range builds {
		name := b.Name
		if name == "" {
			name = emptyValuePlaceholder
		}

		t.Row(
			b.ID,
			name,
			b.ArtifactID,
			b.Status,
			b.CreatedAt.UTC().Format(timestampFormat),
			b.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// formatLogParts renders the "[LEVEL] timestamp message" line shape shared
// by build and workload logs, dropping the timestamp segment when absent.
func formatLogParts(level, timestamp, message string) string {
	if level == "" {
		level = "?"
	}

	if timestamp == "" {
		return fmt.Sprintf("[%s] %s", level, message)
	}

	return fmt.Sprintf("[%s] %s %s", level, timestamp, message)
}

func formatLogLine(entry BuildLogEntry) string {
	return formatLogParts(entry.Levelname, entry.Asctime, entry.Message)
}

func RenderWorkload(format outputformat.OutputFormat, workload Workload) error {
	if format == outputformat.OutputFormatJSON {
		return printWorkloadJSON(workload)
	}

	printWorkloadDetails(workload)

	return nil
}

func RenderWorkloads(format outputformat.OutputFormat, workloads []Workload) error {
	if format == outputformat.OutputFormatJSON {
		return printWorkloadsJSON(workloads)
	}

	printWorkloadsTable(workloads)

	return nil
}

func printWorkloadJSON(workload Workload) error {
	data, err := json.MarshalIndent(NewWorkloadOutput(workload), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printWorkloadsJSON(workloads []Workload) error {
	outputs := make([]WorkloadOutput, 0, len(workloads))

	for _, w := range workloads {
		outputs = append(outputs, NewWorkloadOutput(w))
	}

	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printWorkloadDetails(workload Workload) {
	endpoint := workload.Endpoint
	if endpoint == "" {
		endpoint = emptyValuePlaceholder
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "ID:\t%s\n", workload.ID)
	fmt.Fprintf(w, "Name:\t%s\n", workload.Name)
	fmt.Fprintf(w, "Status:\t%s\n", workload.Status)
	fmt.Fprintf(w, "Endpoint:\t%s\n", endpoint)
	fmt.Fprintf(w, "Type:\t%s\n", workload.Type)
	fmt.Fprintf(w, "Importance:\t%s\n", workload.Importance)
	fmt.Fprintf(w, "Artifact ID:\t%s\n", workload.ArtifactID)
	fmt.Fprintf(w, "Created:\t%s\n", workload.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", workload.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

func printWorkloadsTable(workloads []Workload) {
	if len(workloads) == 0 {
		fmt.Println("No workloads found.")

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"WORKLOAD ID", "NAME", "STATUS", "TYPE", "IMPORTANCE", "UPDATED"}

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

	for _, w := range workloads {
		updated := w.UpdatedAt.UTC().Format(timestampFormat)

		t.Row(w.ID, w.Name, w.Status, w.Type, w.Importance, updated)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

func printArtifactsTable(artifacts []Artifact) {
	if len(artifacts) == 0 {
		fmt.Println("No artifacts found.")

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"ARTIFACT ID", "NAME", "STATUS", "CATALOG ID", "VERSION ID", "UPDATED"}

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

	for _, a := range artifacts {
		catalogID, versionID := emptyValuePlaceholder, emptyValuePlaceholder

		if codeRef := ExtractCodeRef(a); codeRef != nil {
			catalogID = codeRef.CatalogID
			versionID = codeRef.CatalogVersionID
		}

		updated := a.UpdatedAt.UTC().Format(timestampFormat)

		t.Row(a.ID, a.Name, a.Status, catalogID, versionID, updated)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}

// RenderWorkloadOperation renders the acknowledgement of an asynchronous
// start/stop request. Text mode prints the server's human-readable outcome
// message; JSON mode emits the full operation response document so scripts
// keep the workloadId and trackVia handles.
func RenderWorkloadOperation(format outputformat.OutputFormat, resp WorkloadOperationResponse) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(resp)
	}

	fmt.Println(resp.Status)

	return nil
}

// RenderWorkloadStatus renders just the workload's status. Text mode prints
// the bare status value so `dr workload status <id>` is directly usable in
// scripts (symmetric with `dr workload endpoint` printing the bare URL).
func RenderWorkloadStatus(format outputformat.OutputFormat, workload Workload) error {
	if format == outputformat.OutputFormatJSON {
		return printJSON(WorkloadStatusOutput{ID: workload.ID, Status: workload.Status})
	}

	fmt.Println(workload.Status)

	return nil
}

// RenderWorkloadLogs prints a workload's container logs: one
// "[LEVEL] timestamp message" line each, or a JSON array (always [], never
// null, when empty). With no logs in text mode the "No logs found." hint
// goes to stderr, so stdout stays log lines only and a `logs | grep`/pipe is
// not polluted by a status line.
func RenderWorkloadLogs(format outputformat.OutputFormat, entries []WorkloadLogEntry) error {
	if format == outputformat.OutputFormatJSON {
		if entries == nil {
			entries = []WorkloadLogEntry{}
		}

		return printJSON(entries)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No logs found.")

		return nil
	}

	for _, e := range entries {
		fmt.Println(formatWorkloadLogLine(e))
	}

	return nil
}

func formatWorkloadLogLine(e WorkloadLogEntry) string {
	// OTEL levels arrive lowercase (build logs are already uppercase).
	return formatLogParts(strings.ToUpper(e.Level), e.Timestamp, e.Message)
}

// RenderWorkloadLogLine prints a single log entry for the --follow stream:
// the same text line as RenderWorkloadLogs, or one compact JSON object
// (JSON Lines), since a never-ending stream cannot be one closed array.
func RenderWorkloadLogLine(format outputformat.OutputFormat, e WorkloadLogEntry) error {
	if format == outputformat.OutputFormatJSON {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}

		fmt.Println(string(data))

		return nil
	}

	fmt.Println(formatWorkloadLogLine(e))

	return nil
}
