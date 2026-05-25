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
	"strconv"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "get <version-id>",
		Short: "Display details of a single pipeline version",
		Long: `Display the pipeline metadata for a single pipeline version.

Example:
  dr pipeline version get --pipeline <id> 2
  dr pipeline version get --pipeline <id> 2 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			versionID, err := strconv.Atoi(args[0])
			if err != nil || versionID <= 0 {
				return fmt.Errorf("invalid version: %q (expected a positive integer)", args[0])
			}

			result, err := pipeline.GetVersion(pipelineID, versionID)
			if err != nil {
				return handleGetError(err, args[0])
			}

			return pipeline.RenderVersion(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}

func handleGetError(err error, versionLabel string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No version found: " + versionLabel))

		return nil
	}

	return err
}
