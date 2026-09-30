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
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/tui"
)

// RenderWorkloadEventsTo prints a trail to w in the requested format. Under
// JSON the writer receives one {"events": [...]} document, with [] rather
// than null for an empty trail, so a consumer can tell "no events" from
// "nothing was read".
func RenderWorkloadEventsTo(w io.Writer, format outputformat.OutputFormat, events []WorkloadEvent) error {
	if format == outputformat.OutputFormatJSON {
		if events == nil {
			events = []WorkloadEvent{}
		}

		return outputformat.PrintJSONEnvelope(w, "events", events)
	}

	if len(events) == 0 {
		fmt.Fprintln(w, "No events found.")

		return nil
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)
	dimStyle := tui.DimStyle.Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tui.TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return cellStyle.Bold(true)
			case col == 2:
				return dimStyle
			default:
				return cellStyle
			}
		}).
		Headers("TIME", "EVENT", "ACTOR", "DETAILS")

	for _, e := range events {
		t.Row(e.Timestamp.UTC().Format(timestampFormat), e.EventType, orPlaceholder(e.ActorID), eventDetailsCell(e))
	}

	fmt.Fprintln(w, t.String())

	return nil
}

// eventDetailsCell is the platform's sentence for the event when it wrote
// one, and otherwise the details as compact JSON, so nothing the platform
// recorded is hidden behind a dash.
func eventDetailsCell(e WorkloadEvent) string {
	if message := e.Message(); message != "" {
		return message
	}

	if len(e.Details) == 0 || string(e.Details) == "null" {
		return "-"
	}

	return string(e.Details)
}
