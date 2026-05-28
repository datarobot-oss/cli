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
	"github.com/datarobot/cli/cmd/pipeline/fileutil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat pipeline.OutputFormat
		fromFile     string
	)

	cmd := &cobra.Command{
		Use:   "update <pipeline-id> [<file>]",
		Short: "Re-upload a Python file to update a draft pipeline.",
		Long: `Update an existing draft pipeline by re-uploading a Python file.

A new version is appended to the pipeline. The pipeline name encoded in the
uploaded file must match the existing pipeline name. Locked pipelines cannot
be updated.

The path to the Python file can be supplied either as a positional argument
or via the --from-file=<path> flag. Exactly one of the two must be provided.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr pipeline update 507f1f77bcf86cd799439011 ./my_pipeline.py
  dr pipeline update 507f1f77bcf86cd799439011 --from-file=./my_pipeline.py
  dr pipeline update 507f1f77bcf86cd799439011 --from-file=./my_pipeline.py --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, args []string) error {
			pipelineID := args[0]

			filePath, err := fileutil.ResolveFilePath(args[1:], fromFile)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdatePipeline(pipelineID, filePath)
			if err != nil {
				return err
			}

			return pipeline.RenderCreateResponse(outputFormat, *result)
		},
	}

	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the Python file to upload, e.g. --from-file=./my_pipeline.py (alternative to the positional argument)")
	pipeline.AddOutputFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
