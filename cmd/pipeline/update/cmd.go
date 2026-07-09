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
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		imageID      string
		name         string
		description  string
		outputFormat outputformat.OutputFormat
		fromFile     string
	)

	cmd := &cobra.Command{
		Use:   "update <pipeline-id> [<file>]",
		Short: "Update a draft pipeline's file, name, description, or image.",
		Long: `Update an existing draft pipeline. Supports metadata-only updates
(name/description/image) without re-uploading the source file.

When a .py file is provided (positional or --from-file), a new version is
appended. The pipeline name encoded in the uploaded file must match the
existing pipeline name. Locked pipelines cannot be updated.

At least one of a source file (positional argument or --from-file), --name, --description, or --image must be supplied.

Example:
  dr pipeline update 507f1f77bcf86cd799439011 ./my_pipeline.py
  dr pipeline update 507f1f77bcf86cd799439011 --name "My Pipeline"
  dr pipeline update 507f1f77bcf86cd799439011 --description "Trains a model"
  dr pipeline update 507f1f77bcf86cd799439011 --image <image-id>
  dr pipeline update 507f1f77bcf86cd799439011 --name "New Name" --description "New desc"
  dr pipeline update 507f1f77bcf86cd799439011 --from-file=./my_pipeline.py --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		SilenceUsage: true,
		PreRunE:      auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			pipelineID := args[0]

			filePath, err := resolveOptionalFilePath(args[1:], fromFile)
			if err != nil {
				return err
			}

			if filePath == "" && name == "" && description == "" && imageID == "" {
				return errors.New("at least one of a file, --name, --description, or --image must be specified")
			}

			result, err := pipeline.UpdatePipeline(pipelineID, filePath, imageID, name, description)
			if err != nil {
				return err
			}

			return pipeline.RenderCreateResponse(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&imageID, "image", "", "Execution image ID to associate with this pipeline")
	cmd.Flags().StringVar(&name, "name", "", "New display name for the pipeline")
	cmd.Flags().StringVar(&description, "description", "", "New description for the pipeline")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "Path to the Python file to upload (alternative to the positional argument)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

// resolveOptionalFilePath returns the file path from the positional arg or --from-file.
// Returns ("", nil) when neither is provided (metadata-only update).
func resolveOptionalFilePath(args []string, fromFile string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = args[0]
	}

	if positional != "" && fromFile != "" {
		return "", errors.New("specify the file either as a positional argument or via --from-file, not both")
	}

	if positional != "" {
		return positional, nil
	}

	return fromFile, nil
}
