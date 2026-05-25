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

package update

import (
	"errors"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		fromFile     string
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "update <input-id> [<payload-file>]",
		Short: "Update a draft pipeline input set",
		Long: `Update the payload of a draft input set.

Locked inputs are immutable; the API will return 409 if you try to update
one. The new payload must be a JSON object supplied either as a positional
argument or via --from-file=<path>.

Example:
  dr pipeline input update --pipeline <id> <input-id> ./new_payload.json
  dr pipeline input update --pipeline <id> <input-id> --from-file=./new_payload.json --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			inputID := args[0]

			payload, err := pipeline.ResolvePayload(args[1:], fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdateInput(pipelineID, inputID, payload)
			if err != nil {
				return err
			}

			return pipeline.RenderInput(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the JSON payload file, e.g. --from-file=./payload.json (alternative to the positional argument)")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   pipelineID,
			"input_id":      telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
