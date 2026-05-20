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
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/input/inpututil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		pipelineID   string
		fromFile     string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "update <input-id> [<payload-file>]",
		Short: "Update a draft pipeline input set",
		Long: `Update the payload of a draft input set.

Locked inputs are immutable; the API will return 409 if you try to update
one. The new payload must be a JSON object supplied either as a positional
argument or via --from-file=<path>.

Example:
  dr pipelines input update --pipeline <id> <input-id> ./new_payload.json
  dr pipelines input update --pipeline <id> <input-id> --from-file=./new_payload.json --output json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if pipelineID == "" {
				return errors.New("--pipeline is required")
			}

			inputID := args[0]

			payload, err := inpututil.ResolvePayload(args[1:], fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdateInput(pipelineID, inputID, payload)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return inpututil.PrintInputJSON(*result)
			}

			inpututil.PrintInputHuman(*result)

			return nil
		},
	}

	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "Pipeline ID")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the JSON payload file, e.g. --from-file=./payload.json (alternative to the positional argument)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}
