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

// Package scheduleutil holds the rendering helpers shared by the
// `dr pipelines schedule` verbs. Sibling-package layout avoids cycles
// with the parent schedule command.

package scheduleutil

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// PrintScheduleJSON marshals a schedule as indented JSON.
func PrintScheduleJSON(s pipeline.Schedule) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintScheduleHuman renders a single schedule in human-friendly form.
func PrintScheduleHuman(s pipeline.Schedule) {
	fmt.Println(tui.BaseTextStyle.Render("Schedule ID:  " + s.ScheduleID))
	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID:  " + s.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Version:      v" + strconv.Itoa(s.Version)))
	fmt.Println(tui.BaseTextStyle.Render("Cron:         " + s.CronExpression))
	fmt.Println(tui.BaseTextStyle.Render("Timezone:     " + s.Timezone))
	fmt.Println(tui.BaseTextStyle.Render("Status:       " + string(s.Status)))
	fmt.Println(tui.DimStyle.Render("Created:      " + s.CreatedAt))
	fmt.Println(tui.DimStyle.Render("Updated:      " + s.UpdatedAt))
}

// PrintScheduleListJSON marshals a list of schedules as indented JSON.
func PrintScheduleListJSON(items []pipeline.Schedule) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintScheduleListHuman renders a tabular summary of schedules.
func PrintScheduleListHuman(items []pipeline.Schedule) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No schedules found"))

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "SCHEDULE_ID\tVERSION\tCRON\tTIMEZONE\tSTATUS\tUPDATED")

	for _, s := range items {
		fmt.Fprintf(writer, "%s\tv%d\t%s\t%s\t%s\t%s\n",
			s.ScheduleID, s.Version, s.CronExpression, s.Timezone, s.Status, s.UpdatedAt,
		)
	}

	_ = writer.Flush()
}
