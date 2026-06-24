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
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		rawPackages  []string
		outputFormat outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "update <image-id>",
		Short: "Add a new version to a pipeline execution image",
		Long: `Update a pipeline execution image by creating a new version with the supplied pip packages.

Each update creates a new immutable version. The supplied packages become
the complete pip list for that version — existing packages are not carried
over automatically. To add a package, include all desired packages in the
new version's list.

Example:
  dr pipeline image update img-123 --package scikit-learn
  dr pipeline image update img-123 --package "scikit-learn==1.5,torch" --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			packages, err := pipeline.NormalizePackages(rawPackages)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdateImage(args[0], packages)
			if err != nil {
				return err
			}

			return pipeline.RenderImage(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"image_id":      telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
