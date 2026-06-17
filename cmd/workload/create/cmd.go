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
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var specFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create (deploy) a workload.",
		Long: `Create a workload in your DataRobot deployment infrastructure.

This command reads a JSON or YAML spec from a file and POSTs it to the
Workload API. The created workload is returned and shown, including its
stable endpoint URL. Startup is asynchronous: poll with
'dr workload get <id>' until the status is "running", then call the
endpoint.

The spec requires name and exactly one of artifactId / artifact. Any other
field accepted by the server is accepted here. JSON specs are sent to the
Workload API verbatim, byte-for-byte. YAML specs are converted to JSON
using standard YAML typing rules before sending: quote values that must
stay strings (for example "0644" or "1.10"), and unquoted dates are sent
as RFC3339 timestamps. The server validates field-level shape and returns
a 422 with a JSON-path detail on a mismatch.

Two flows:

  1. Deploy an existing artifact (e.g. one built with 'dr artifact code sync'
     and a build):

  {
    "name": "my-app",
    "artifactId": "68b0c1d2e3f4a5b6c7d8e9f0",
    "runtime": {
      "containerGroups": [{
        "name": "default",
        "replicaCount": 1,
        "containers": [{
          "name": "primary",
          "resourceAllocation": {"cpu": 1, "memory": "512MB"}
        }]
      }]
    }
  }

  2. Define a draft artifact inline and deploy it in one call:

  {
    "name": "hello-whoami",
    "artifact": {
      "name": "whoami-artifact",
      "type": "service",
      "spec": {
        "containerGroups": [{
          "name": "default",
          "containers": [{
            "name": "whoami",
            "imageUri": "containous/whoami:latest",
            "port": 8080,
            "primary": true
          }]
        }]
      }
    }
  }

Example:
  dr workload create --spec-file workload.json
  dr workload create --spec-file workload.yaml
  dr workload create --spec-file workload.yaml --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			payload, err := workload.ReadSpecFile(specFile)
			if err != nil {
				return err
			}

			if err := workload.ValidateWorkloadCreateRequest(payload); err != nil {
				return err
			}

			wl, err := workload.CreateWorkload(payload)
			if err != nil {
				return err
			}

			return workload.RenderWorkload(outputFormat, *wl)
		},
	}

	cmd.Flags().StringVar(&specFile, "spec-file", "", "Path to JSON or YAML spec file (required)")
	_ = cmd.MarkFlagRequired("spec-file")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format": string(outputformat.GetFormat(cmd)),
		}
	})

	return cmd
}
