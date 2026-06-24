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

package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		flags        scopeflag.Flags
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "source",
		Short: "Display the source code of a pipeline",
		Long: `Display the full source.py content of a pipeline.

Scope is selected from the --scope/--version flags:
  - no flags                   -> draft source (latest version)
  - --version=N                -> locked source for version N (scope auto-set)
  - --scope=draft              -> draft source
  - --scope=locked --version=N -> locked source for version N

Example:
  dr pipeline source --pipeline <id>
  dr pipeline source --pipeline <id> --version=2
  dr pipeline source --pipeline <id> --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			result, err := pipeline.GetPipelineSource(flags.PipelineID, scope, version)
			if err != nil {
				return handleSourceError(err, flags.PipelineID)
			}

			if outputFormat == outputformat.OutputFormatJSON {
				return printSourceJSON(*result)
			}

			fmt.Print(result.Source)

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	flags.Bind(cmd)
	_ = cmd.MarkFlagRequired("pipeline")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"pipeline_id":   flags.PipelineID,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func handleSourceError(err error, pipelineID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No source available for pipeline: " + pipelineID))

		return nil
	}

	return err
}

func printSourceJSON(s pipeline.PipelineSourceResponse) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))

	return nil
}
