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
		baseImage     string
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
  dr pipeline image create --name ml-base --conda scipy --conda numpy --base-image python:3.12
  dr pipeline image create --name gpu-base --package torch --nvidia --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			if name == "" {
				return errors.New("--name is required")
			}

			pip := pipeline.NormalizePackageList(rawPackages)
			conda := pipeline.BuildCondaValue(rawConda, rawCondaChans)

			if len(pip) == 0 && conda == nil {
				return errors.New("at least one of --package (pip) or --conda is required")
			}

			if conda != nil && len(conda.Deps) == 0 {
				return errors.New("--conda-channel requires at least one --conda package")
			}

			result, err := pipeline.CreateImage(name, description, pip, conda, baseImage, nvidia)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&baseImage, "base-image", "", "Docker base image URI (e.g. python:3.12)")
	cmd.Flags().BoolVar(&nvidia, "nvidia", false, "Enable NVIDIA GPU support")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
