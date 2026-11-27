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

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		version      int
		payloadOnly  bool
	)

	cmd := &cobra.Command{
		Use:   "get <recipe-id>",
		Short: "Show a recipe with its content.",
		Long: `Show a recipe: its header, validation diagnostics and descriptor.

--payload prints only the payload, as YAML (JSON with --output-format json),
ready to edit and pass back to 'dr recipe patch'.

Example:
  dr recipe get 6a1f0c2e9b7d4e3f2a1b0c9d
  dr recipe get 6a1f0c2e9b7d4e3f2a1b0c9d --version 2
  dr recipe get 6a1f0c2e9b7d4e3f2a1b0c9d --payload > recipe.yaml`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			r, err := llm.GetRecipe(args[0], version)
			if err != nil {
				return notFound(cmd, err, args[0], outputFormat)
			}

			if payloadOnly {
				return llm.RenderPayload(cmd.OutOrStdout(), outputFormat, *r)
			}

			return llm.RenderRecipe(cmd.OutOrStdout(), outputFormat, *r)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().Var(countflags.NonNegativeInt(&version, 0), "version", "A past version of the content (default: latest)")
	cmd.Flags().BoolVar(&payloadOnly, "payload", false, "Print only the payload")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"version_given": version > 0,
			"payload":       payloadOnly,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

// notFound turns a 404 into an informational line (exit 0), as `dr pipeline
// get` does; JSON mode keeps the error so stdout stays pure JSON.
func notFound(cmd *cobra.Command, err error, id string, format outputformat.OutputFormat) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound && format != outputformat.OutputFormatJSON {
		fmt.Fprintln(cmd.OutOrStdout(), tui.DimStyle.Render("No recipe found with id: "+id))

		return nil
	}

	return err
}
