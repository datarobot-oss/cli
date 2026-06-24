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
		rawPackages   []string
		rawConda      []string
		rawCondaChans []string
		baseImage     string
		nvidia        bool
		outputFormat  outputformat.OutputFormat
	)

	cmd := &cobra.Command{
		Use:   "update <image-id>",
		Short: "Add a new version to a pipeline execution image",
		Long: `Update a pipeline execution image by creating a new version with the supplied definition.

Each update creates a new immutable version. The supplied definition becomes
the complete spec for that version — existing packages are not carried over
automatically. To add a package, include all desired packages in the new version.

At least one of --package (pip) or --conda must be provided.

Example:
  dr pipeline image update img-123 --package scikit-learn
  dr pipeline image update img-123 --conda scipy --conda numpy
  dr pipeline image update img-123 --package torch --nvidia --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			pip := pipeline.NormalizePackageList(rawPackages)
			conda := pipeline.BuildCondaValue(rawConda, rawCondaChans)

			if len(pip) == 0 && conda == nil {
				return errors.New("at least one of --package (pip) or --conda is required")
			}

			if conda != nil && len(conda.Deps) == 0 {
				return errors.New("--conda-channel requires at least one --conda package")
			}

			result, err := pipeline.UpdateImage(args[0], pip, conda, baseImage, nvidia)
			if err != nil {
				return err
			}

			return pipeline.RenderImage(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")
	cmd.Flags().StringSliceVar(&rawConda, "conda", nil, "Conda package spec (repeatable)")
	cmd.Flags().StringSliceVar(&rawCondaChans, "conda-channel", nil, "Conda channel (repeatable; if set, sends a structured CondaSpec)")
	cmd.Flags().StringVar(&baseImage, "base-image", "", "Docker base image URI (e.g. python:3.12)")
	cmd.Flags().BoolVar(&nvidia, "nvidia", false, "Enable NVIDIA GPU support")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"image_id":      telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
