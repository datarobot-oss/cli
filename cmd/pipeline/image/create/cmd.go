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
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		name          string
		description   string
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
		Use:   "create",
		Short: "Create a pipeline execution image",
		Long: `Create a new pipeline execution image.

A new image is registered with an initial version (v1) containing the
supplied package definition. The image may be referenced by pipelines once
its first version reaches the READY state.

At least one of --package (pip) or --conda must be provided.

Example:
  dr pipeline image create --name ml-base --package numpy --package pandas
  dr pipeline image create --name py311 --package numpy --python-version 3.11
  dr pipeline image create --name gpu-base --package torch --gpu --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			// --name is enforced by MarkFlagRequired below; Cobra rejects a
			// missing required flag before RunE runs, so no manual check here.
			pip := pipeline.NormalizePackageList(rawPackages)
			conda := pipeline.BuildCondaValue(rawConda, rawCondaChans)

			if len(pip) == 0 && conda == nil {
				return errors.New("at least one of --package (pip) or --conda is required")
			}

			if conda != nil && len(conda.Deps) == 0 {
				return errors.New("--conda-channel requires at least one --conda package")
			}

			// --gpu is canonical; --nvidia is a deprecated alias for it.
			result, err := pipeline.CreateImage(name, description, pip, conda, pythonVersion, baseImage, gpu || nvidia)
			if err != nil {
				return fmt.Errorf("create image: %w", err)
			}

			return pipeline.RenderImage(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&name, "name", "", "Image name (required)")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")
	cmd.Flags().StringSliceVar(&rawConda, "conda", nil, "Conda package spec (repeatable)")
	cmd.Flags().StringSliceVar(&rawCondaChans, "conda-channel", nil, "Conda channel (repeatable; if set, sends a structured CondaSpec)")
	cmd.Flags().StringVar(&pythonVersion, "python-version", "", "Python interpreter version, e.g. 3.11 (allowed: 3.10-3.13)")
	cmd.Flags().StringVar(&baseImage, "base-image", "", "DEPRECATED: use --python-version. A bare version (e.g. 3.11) is accepted; a full image reference is ignored at build time")
	cmd.Flags().BoolVar(&gpu, "gpu", false, "Enable GPU support (rejected with 422 if the environment has no GPU capacity)")
	cmd.Flags().BoolVar(&nvidia, "nvidia", false, "DEPRECATED: use --gpu")
	_ = cmd.Flags().MarkDeprecated("nvidia", "use --gpu")

	// --base-image is the deprecated way to pick a Python interpreter; passing
	// it together with the canonical --python-version would send two competing
	// values on the wire, so reject the combination up front.
	cmd.MarkFlagsMutuallyExclusive("python-version", "base-image")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
