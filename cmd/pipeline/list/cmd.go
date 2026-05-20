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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		mode         string
		offset       int
		limit        int
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pipeline.",
		Long: `List pipelines registered with the pipelines service.

By default, output is human-readable. Use --output json for machine-parseable output.

Example:
  dr pipelines list
  dr pipelines list --mode draft
  dr pipelines list --offset 0 --limit 50 --output json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if mode != "" && mode != pipeline.ModeDraft && mode != pipeline.ModeLocked {
				return fmt.Errorf("invalid mode: %s (supported: draft, locked)", mode)
			}

			list, err := pipeline.ListPipelines(mode, offset, limit)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return printListJSON(*list)
			}

			printListHuman(*list)

			return nil
		},
	}

	cmd.Flags().StringVar(&mode, "mode", "", "Filter by mode: draft or locked")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 50, "Pagination limit (1-200)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}

func printListJSON(list pipeline.ListResponse) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printListHuman(list pipeline.ListResponse) {
	if len(list.Items) == 0 {
		fmt.Println(tui.DimStyle.Render("No pipelines found."))

		return
	}

	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("Showing %d of %d (offset=%d limit=%d)", len(list.Items), list.Total, list.Offset, list.Limit)))
	fmt.Println()

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "ID\tNAME\tMODE\tACTIVE\tVERSION\tUPDATED")

	for _, item := range list.Items {
		latest := "\u2014"
		if item.LatestVersion != nil {
			latest = "v" + strconv.Itoa(*item.LatestVersion)
		}

		updated := item.UpdatedAt.UTC().Format(time.RFC3339)
		active := strconv.FormatBool(item.IsActive)

		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			item.PipelineID, item.Name, item.Mode, active, latest, updated)
	}

	_ = writer.Flush()
}
