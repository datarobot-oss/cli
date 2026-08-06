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
		pythonVersion string
		baseImage     string
		gpu           bool
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
  dr pipeline image update img-123 --package numpy --python-version 3.11
  dr pipeline image update img-123 --package torch --gpu --output-format json`,
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

			// --gpu is canonical; --nvidia is a deprecated alias for it.
			result, err := pipeline.UpdateImage(args[0], pip, conda, pythonVersion, baseImage, gpu || nvidia)
			if err != nil {
				return fmt.Errorf("update image: %w", err)
			}

			return pipeline.RenderImage(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")
	cmd.Flags().StringSliceVar(&rawConda, "conda", nil, "Conda package spec (repeatable)")
	cmd.Flags().StringSliceVar(&rawCondaChans, "conda-channel", nil, "Conda channel (repeatable; if set, sends a structured CondaSpec)")
	cmd.Flags().StringVar(&pythonVersion, "python-version", "", "Python interpreter version, e.g. 3.11 (allowed: 3.10-3.13)")
	cmd.Flags().StringVar(&baseImage, "base-image", "", "DEPRECATED: use --python-version. A bare version (e.g. 3.11) is accepted; a full image reference is ignored at build time")
	cmd.Flags().BoolVar(&gpu, "gpu", false, "Enable GPU support (rejected with 422 if the environment has no GPU capacity)")
	cmd.Flags().BoolVar(&nvidia, "nvidia", false, "DEPRECATED: use --gpu")
	_ = cmd.Flags().MarkDeprecated("nvidia", "use --gpu")

	telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"image_id":      telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
