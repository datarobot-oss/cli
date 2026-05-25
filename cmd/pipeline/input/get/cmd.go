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
		Use:   "get <input-id>",
		Short: "Display details of a pipeline input set",
		Long: `Display the full payload and metadata for a single input set.

Example:
  dr pipeline input get --pipeline <id> <input-id>
  dr pipeline input get --pipeline <id> --version=2 <input-id> --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.PipelineID == "" {
				return errors.New("--pipeline is required")
			}

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			result, err := pipeline.GetInput(flags.PipelineID, scope, version, args[0])
			if err != nil {
				return handleGetError(err, args[0])
			}

			return pipeline.RenderInput(outputFormat, *result)
		},
	}

	flags.Bind(cmd)
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}

func handleGetError(err error, inputID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No input found with id: " + inputID))

		return nil
	}

	return err
}
