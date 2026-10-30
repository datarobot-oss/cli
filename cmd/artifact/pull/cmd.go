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

package pull

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/features"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/docker"
	"github.com/spf13/cobra"
)

// Seams swapped by tests.
var (
	getArtifactFn = workload.GetArtifact
	pullFn        = docker.Pull
	apiTokenFn    = func() string { return viperx.GetString(config.DataRobotAPIKey) }
)

// pulled is the --output-format json shape.
type pulled struct {
	ArtifactID string `json:"artifactId"`
	Image      string `json:"image"`
	Digest     string `json:"digest"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "pull <artifact-id>",
		Short: "Pull an artifact's pushed image with the local Docker",
		Long: `Pull the image an artifact runs, when that image was pushed to DataRobot
(for example by 'dr workload up' building it with Docker).

A locked artifact names one exact image, which is pulled by its digest and
tagged <registry>/artifacts/<artifact-id>. A draft runs whatever was pushed to
it last, which is what is pulled. Docker logs in with your DataRobot API key
for this command only; your Docker config is not changed.

Images built on the platform and published images are not pulled through
DataRobot; pull those from their own registry.

Example:
  dr artifact pull 68b0c1d2e3f4a5b6c7d8e9f0
  dr artifact pull 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			artifact, err := getArtifactFn(args[0])
			if err != nil {
				return err
			}

			ref, ok := docker.ParseReference(workload.GetPrimaryContainerImageURI(*artifact))
			if !ok {
				return fmt.Errorf("the image of artifact %s was built on the platform or is a published image; "+
					"nothing to pull through DataRobot", args[0])
			}

			stderr := cmd.ErrOrStderr()

			image, digest, err := pullFn(cmd.Context(), ref, apiTokenFn(), func(line string) {
				fmt.Fprintln(stderr, "  "+line)
			})
			if err != nil {
				return err
			}

			if outputFormat == outputformat.OutputFormatJSON {
				return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "pull",
					pulled{ArtifactID: args[0], Image: image, Digest: digest})
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), image)

			return err
		},
	}

	features.SetGate(cmd, features.LocalBuild)
	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
