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

// schedule_output.go holds the rendering helpers shared by the
// `dr pipelines schedule` verbs.
package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

// scheduleJSON is the CLI-facing DTO used for `--output-format json`.
type scheduleJSON struct {
	ScheduleID     string `json:"schedule_id"`
	PipelineID     string `json:"pipeline_id"`
	Version        int    `json:"version"`
	CronExpression string `json:"cron_expression"`
	Timezone       string `json:"timezone"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toScheduleJSON(s Schedule) scheduleJSON {
	return scheduleJSON{
		ScheduleID:     s.ScheduleID,
		PipelineID:     s.PipelineID,
		Version:        s.Version,
		CronExpression: s.CronExpression,
		Timezone:       s.Timezone,
		Status:         string(s.Status),
		CreatedAt:      s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RenderSchedule routes a single schedule to JSON or human output.
func RenderSchedule(format OutputFormat, s Schedule) error {
	if format == OutputFormatJSON {
		return PrintScheduleJSON(s)
	}

	PrintScheduleHuman(s)

	return nil
}

// RenderSchedules routes a list of schedules to JSON or human output.
func RenderSchedules(format OutputFormat, items []Schedule) error {
	if format == OutputFormatJSON {
		return PrintScheduleListJSON(items)
	}

	PrintScheduleListHuman(items)

	return nil
}

// PrintScheduleJSON marshals a schedule as indented JSON through the DTO.
func PrintScheduleJSON(s Schedule) error {
	data, err := json.MarshalIndent(toScheduleJSON(s), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintScheduleHuman renders a single schedule in human-friendly form.
func PrintScheduleHuman(s Schedule) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Schedule ID:\t%s\n", s.ScheduleID)
	fmt.Fprintf(w, "Pipeline ID:\t%s\n", s.PipelineID)
	fmt.Fprintf(w, "Version:\tv%d\n", s.Version)
	fmt.Fprintf(w, "Cron:\t%s\n", s.CronExpression)
	fmt.Fprintf(w, "Timezone:\t%s\n", s.Timezone)
	fmt.Fprintf(w, "Status:\t%s\n", string(s.Status))
	fmt.Fprintf(w, "Created:\t%s\n", s.CreatedAt.UTC().Format(timestampFormat))
	fmt.Fprintf(w, "Updated:\t%s\n", s.UpdatedAt.UTC().Format(timestampFormat))

	w.Flush()
}

// PrintScheduleListJSON marshals a list of schedules as indented JSON through the DTO.
func PrintScheduleListJSON(items []Schedule) error {
	view := make([]scheduleJSON, len(items))

	for i, s := range items {
		view[i] = toScheduleJSON(s)
	}

	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintScheduleListHuman renders a lipgloss table summary of schedules.
func PrintScheduleListHuman(items []Schedule) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No schedules found"))

		return
	}

	cellStyle := tui.BaseTextStyle.Padding(0, 1)

	dimStyle := tui.DimStyle.Padding(0, 1)

	headers := []string{"SCHEDULE ID", "VERSION", "CRON", "TIMEZONE", "STATUS", "UPDATED"}

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

	for _, s := range items {
		t.Row(
			s.ScheduleID,
			fmt.Sprintf("v%d", s.Version),
			s.CronExpression,
			s.Timezone,
			string(s.Status),
			s.UpdatedAt.UTC().Format(timestampFormat),
		)
	}

	fmt.Fprintln(os.Stdout, t.Render())
}
