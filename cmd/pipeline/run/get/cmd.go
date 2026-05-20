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

package get

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/pipeline/run/runutil"
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
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "get <run-id>",
		Short: "Display details of a pipeline run",
		Long: `Display the full record for a single run.

Example:
  dr pipelines run get --pipeline <id> <run-id>
  dr pipelines run get --pipeline <id> --version=2 <run-id> --output json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if flags.PipelineID == "" {
				return errors.New("--pipeline is required")
			}

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			result, err := pipeline.GetRun(flags.PipelineID, scope, version, args[0])
			if err != nil {
				return handleGetError(err, args[0])
			}

			if outputFormat == "json" {
				return runutil.PrintRunJSON(*result)
			}

			runutil.PrintRunHuman(*result)

			return nil
		},
	}

	flags.Bind(cmd)
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}

func handleGetError(err error, runID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No run found with id: " + runID))

		return nil
	}

	return err
}
