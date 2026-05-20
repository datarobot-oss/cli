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

package lock

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "lock <pipeline-id>",
		Short: "Lock a draft pipeline",
		Long: `Promote a draft pipeline to locked mode. Once locked, the pipeline can
no longer be updated and locked runs/inputs/schedules become valid.

Example:
  dr pipelines lock 507f1f77bcf86cd799439011
  dr pipelines lock 507f1f77bcf86cd799439011 --output json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			result, err := pipeline.LockPipeline(args[0])
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return printLockJSON(*result)
			}

			printLockHuman(*result)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}

func printLockJSON(result pipeline.CreateResponse) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printLockHuman(result pipeline.CreateResponse) {
	tasks := "\u2014"
	if len(result.TaskNames) > 0 {
		tasks = strings.Join(result.TaskNames, ", ")
	}

	fmt.Println(tui.BaseTextStyle.Render("Pipeline ID:  " + result.PipelineID))
	fmt.Println(tui.BaseTextStyle.Render("Name:         " + result.Name))
	fmt.Println(tui.BaseTextStyle.Render("Mode:         " + result.Mode))
	fmt.Println(tui.BaseTextStyle.Render("Version:      v" + strconv.Itoa(result.Version)))
	fmt.Println(tui.BaseTextStyle.Render("Status:       " + result.Status))
	fmt.Println(tui.BaseTextStyle.Render("Tasks:        " + tasks))
	fmt.Println(tui.DimStyle.Render("Locked:       " + result.CreatedAt.UTC().Format(time.RFC3339)))
}
