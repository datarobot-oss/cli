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

// Package versionutil contains rendering helpers shared by the
// `dr pipelines version` verbs. Living in a sibling package keeps the
// parent `version` package free of cycles.

package versionutil

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
)

// PrintVersionJSON marshals a single version as indented JSON.
func PrintVersionJSON(v pipeline.PipelineVersion) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintVersionHuman renders the key facts about a single version.
func PrintVersionHuman(v pipeline.PipelineVersion) {
	tasks := "\u2014"
	if len(v.TaskNames) > 0 {
		tasks = strings.Join(v.TaskNames, ", ")
	}

	python := v.PythonVersion
	if python == "" {
		python = "\u2014"
	}

	fmt.Println(tui.BaseTextStyle.Render("Version:        v" + strconv.Itoa(v.Version)))
	fmt.Println(tui.BaseTextStyle.Render("Pipeline:       " + v.PipelineName))
	fmt.Println(tui.BaseTextStyle.Render("Status:         " + v.Status))
	fmt.Println(tui.BaseTextStyle.Render("Python Version: " + python))
	fmt.Println(tui.BaseTextStyle.Render("Tasks:          " + tasks))

	if v.ErrorDetail != "" {
		fmt.Println(tui.BaseTextStyle.Render("Error:          " + v.ErrorDetail))
	}

	fmt.Println(tui.DimStyle.Render("Created:        " + v.CreatedAt.UTC().Format(time.RFC3339)))
}

// PrintVersionListJSON marshals a list of versions as indented JSON.
func PrintVersionListJSON(items []pipeline.PipelineVersion) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

// PrintVersionListHuman renders a tabular summary of versions.
func PrintVersionListHuman(items []pipeline.PipelineVersion) {
	if len(items) == 0 {
		fmt.Println(tui.DimStyle.Render("No versions found"))

		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "VERSION\tSTATUS\tPYTHON\tCREATED\tTASKS")

	for _, v := range items {
		tasks := "\u2014"
		if len(v.TaskNames) > 0 {
			tasks = strings.Join(v.TaskNames, ", ")
		}

		python := v.PythonVersion
		if python == "" {
			python = "\u2014"
		}

		fmt.Fprintf(writer, "v%s\t%s\t%s\t%s\t%s\n",
			strconv.Itoa(v.Version),
			v.Status,
			python,
			v.CreatedAt.UTC().Format(time.RFC3339),
			tasks,
		)
	}

	_ = writer.Flush()
}
