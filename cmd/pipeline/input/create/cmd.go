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

package create

import (
	"errors"

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		flags        scopeflag.Flags
		fromFile     string
		outputFormat pipeline.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "create [<payload-file>]",
		Short: "Create a pipeline input set",
		Long: `Create an input payload for a pipeline.

The payload must be a JSON object. The path to the JSON file can be
supplied either as a positional argument or via --from-file=<path>.
Exactly one of the two must be provided.

Scope is selected from the --scope/--version flags:
  - no flags                  -> draft
  - --version=N               -> locked, version N (scope auto-set)
  - --scope=draft             -> draft
  - --scope=locked --version=N -> locked, version N

Example:
  dr pipeline input create --pipeline <id> ./payload.json
  dr pipeline input create --pipeline <id> --from-file=./payload.json
  dr pipeline input create --pipeline <id> --version=2 ./payload.json --output-format json`,
		Args:         cobra.MaximumNArgs(1),
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

			payload, err := pipeline.ResolvePayload(args, fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.CreateInput(flags.PipelineID, scope, version, payload)
			if err != nil {
				return err
			}

			return pipeline.RenderInput(outputFormat, *result)
		},
	}

	flags.Bind(cmd)
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the JSON payload file, e.g. --from-file=./payload.json (alternative to the positional argument)")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	return cmd
}
