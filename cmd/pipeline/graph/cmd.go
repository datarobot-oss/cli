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

package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		flags        scopeflag.Flags
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Display the DAG of a pipeline",
		Long: `Display the pipeline/task graph (DAG) as either a JSON payload
(for visualisation tooling) or a human-readable summary.

Scope is selected from the --scope/--version flags:
  - no flags                   -> draft graph (latest version)
  - --version=N                -> locked graph for version N (scope auto-set)
  - --scope=draft              -> draft graph
  - --scope=locked --version=N -> locked graph for version N

Example:
  dr pipeline graph --pipeline <id>
  dr pipeline graph --pipeline <id> --version=2 --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flags.PipelineID == "" {
				return errors.New("--pipeline is required")
			}

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			result, err := pipeline.GetGraph(flags.PipelineID, scope, version)
			if err != nil {
				return handleGraphError(err, flags.PipelineID)
			}

			if outputFormat == pipeline.OutputFormatJSON {
				return printGraphJSON(*result)
			}

			printGraphHuman(*result)

			return nil
		},
	}

	flags.Bind(cmd)
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}

func handleGraphError(err error, pipelineID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No graph available for pipeline: " + pipelineID))

		return nil
	}

	return err
}

func printGraphJSON(g pipeline.Graph) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}

func printGraphHuman(g pipeline.Graph) {
	fmt.Println(tui.BaseTextStyle.Render("Pipeline: " + g.Pipeline.Name))

	if len(g.Nodes) == 0 {
		fmt.Println(tui.DimStyle.Render("No nodes"))

		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Nodes (" + strconv.Itoa(len(g.Nodes)) + "):"))

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "  ID\tTYPE\tNAME")

	for _, n := range g.Nodes {
		fmt.Fprintf(writer, "  %d\t%s\t%s\n", n.ID, n.Type, n.Name)
	}

	_ = writer.Flush()

	if len(g.Edges) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("Edges (" + strconv.Itoa(len(g.Edges)) + "):"))

	writer = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintln(writer, "  SOURCE\tTARGET")

	for _, e := range g.Edges {
		fmt.Fprintf(writer, "  %d\t%d\n", e.Source, e.Target)
	}

	_ = writer.Flush()
}
