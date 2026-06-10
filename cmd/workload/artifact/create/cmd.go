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
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat workload.OutputFormat

	var specFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workload artifact.",
		Long: `Create a workload artifact in your DataRobot deployment infrastructure.

This command reads a JSON or YAML spec from a file and POSTs it to the
Workload API. The created artifact is returned and shown.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Required fields: name, spec.containerGroups (>=1), and at least one container
per group. Any other field accepted by the server is accepted here. JSON
specs are sent to the Workload API verbatim, byte-for-byte. YAML specs are
converted to JSON using standard YAML typing rules before sending: quote
values that must stay strings (for example "0644" or "1.10"), unquoted
dates are sent as RFC3339 timestamps, and only the first document of a
multi-document file is read. The server validates field-level shape and
returns a 422 with a JSON-path detail on a mismatch.

Container lifecycles:

  1. Prebuilt image: set imageUri (+ port + primary on the entry container).
  2. Build from source: set imageBuildConfig.dockerfile.source = "provided"
     to build from ./Dockerfile in your code, or "generated" together with
     executionEnvironmentId, executionEnvironmentVersionId, and entrypoint
     to have the server generate a Dockerfile from a base image.
     'dr workload code sync' fills in imageBuildConfig.codeRef after upload.

Minimal prebuilt example:

  {
    "name": "my-agent",
    "spec": {
      "containerGroups": [{
        "containers": [{
          "imageUri": "nginx:latest",
          "port": 8080,
          "primary": true
        }]
      }]
    }
  }

Minimal build-from-source example (provided Dockerfile):

  {
    "name": "my-agent",
    "spec": {
      "containerGroups": [{
        "containers": [{
          "primary": true,
          "port": 8080,
          "imageBuildConfig": { "dockerfile": { "source": "provided" } }
        }]
      }]
    }
  }

Example:
  dr workload artifact create --spec-file spec.json
  dr workload artifact create --spec-file spec.yaml
  dr workload artifact create --spec-file spec.yaml --output-format json`,
		Args:    cobra.NoArgs,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(_ *cobra.Command, _ []string) error {
			payload, err := workload.ReadSpecFile(specFile)
			if err != nil {
				return err
			}

			if err := workload.ValidateCreateRequest(payload); err != nil {
				return err
			}

			var artifact *workload.Artifact

			if err := tui.RunWithSpinner("Creating artifact…", func() error {
				var createErr error

				artifact, createErr = workload.CreateArtifact(payload)

				return createErr
			}); err != nil {
				return err
			}

			return workload.RenderArtifact(outputFormat, *artifact)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	cmd.Flags().StringVar(&specFile, "spec-file", "", "Path to JSON or YAML spec file (required)")
	_ = cmd.MarkFlagRequired("spec-file")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
