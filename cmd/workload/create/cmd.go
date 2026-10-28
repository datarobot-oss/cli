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
	"encoding/json"
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		specFile  string
		enclave   string
		useCaseID string
	)

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

--use-case-id <id> links the workload to a Use Case at create time. On its
own it does not place the workload on an Enclave: without a selection
policy the workload runs outside any Enclave, like any other asset in the
Use Case. To place it on the Enclaves an administrator has granted to the
Use Case, set the policy in the spec:

  "runtime": {"enclaveSelectionPolicy": "availability", ...}

and DataRobot picks among the granted Enclaves. If the Use Case has
Enclaves and the spec sets no policy, the server refuses the create with
ENCLAVE_TARGETING_REQUIRED. Add --enclave <name> to pin one specific
Enclave instead (the policy becomes "manual"); pinning requires the
CAN_OVERRIDE_WORKLOAD_PLACEMENT permission on workloads, and the pinned
Enclave must be granted to the Use Case. --enclave needs a Use Case, from
--use-case-id or useCaseId in the spec. Without a policy or --enclave the
workload is not placed on an Enclave. The flags refuse to override a spec
that already sets the fields they write, and using them means the spec is
re-encoded rather than sent byte-for-byte. Confirm where the workload
landed with 'dr workload list --enclave <name>'.

Three flows:

  1. Deploy an existing artifact with a fixed replica count (e.g. one built with
     'dr artifact code sync' and a build):

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

  2. Deploy with autoscaling (replica bounds on autoscaling, not per policy).
     Use replicaCount OR autoscaling.enabled=true per container group, not both.
     See docs/examples/workload-autoscaling.yaml.

  {
    "name": "my-app",
    "artifactId": "68b0c1d2e3f4a5b6c7d8e9f0",
    "runtime": {
      "containerGroups": [{
        "name": "default",
        "autoscaling": {
          "enabled": true,
          "minReplicaCount": 1,
          "maxReplicaCount": 10,
          "policies": [{
            "scalingMetric": "cpuAverageUtilization",
            "target": 80
          }]
        },
        "containers": [{
          "name": "primary",
          "resourceAllocation": {"cpu": 1, "memory": "512MB"}
        }]
      }]
    }
  }

  3. Define a draft artifact inline and deploy it in one call:

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
  dr workload create --spec-file workload.json --use-case-id 68b0aa11bb22cc33dd44ee55
  dr workload create --spec-file workload.json --use-case-id 68b0aa11bb22cc33dd44ee55 --enclave prod-east
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

			payload, err = applyPlacementFlags(cmd, payload, enclave, useCaseID)
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

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&specFile, "spec-file", "", "Path to JSON or YAML spec file (required)")
	_ = cmd.MarkFlagRequired("spec-file")

	cmd.Flags().StringVar(&enclave, "enclave", "",
		"Pin the workload to the named Enclave (sets runtime.enclaveSelectionPolicy=manual); requires --use-case-id")

	cmd.Flags().StringVar(&useCaseID, "use-case-id", "",
		"Link the workload to this Use Case; set runtime.enclaveSelectionPolicy in the spec to place it on the Use Case's Enclaves")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		// No raw ids: whether each flag was given, and the placement they ask for.
		return map[string]any{
			"output_format":        string(outputFormat),
			"use_case_id_provided": cmd.Flags().Changed("use-case-id"),
			"enclave_provided":     cmd.Flags().Changed("enclave"),
			"placement_mode":       placementMode(cmd),
		}
	})

	return cmd
}

// applyPlacementFlags applies --enclave and --use-case-id to the create spec.
func applyPlacementFlags(
	cmd *cobra.Command, payload json.RawMessage, enclave, useCaseID string,
) (json.RawMessage, error) {
	var err error

	// The server requires a Use Case for any Enclave-targeted create;
	// fail here instead of a guaranteed round-trip rejection. A spec
	// that already names one satisfies the requirement.
	if !workload.SpecSetsUseCase(payload) {
		if err := cli.RequireFlagWhenChanged(cmd, "enclave", "use-case-id"); err != nil {
			return nil, fmt.Errorf(
				"%w (or useCaseId in the spec): Enclave placement is governed by a Use Case", err)
		}
	}

	// Changed, not enclave != "": an explicit --enclave "" must be
	// rejected by ApplyEnclavePin, not silently create unpinned.
	if cmd.Flags().Changed("enclave") {
		payload, err = workload.ApplyEnclavePin(payload, enclave)
		if err != nil {
			return nil, err
		}
	}

	if cmd.Flags().Changed("use-case-id") {
		id, err := usecase.ParseID(useCaseID)
		if err != nil {
			return nil, fmt.Errorf("invalid --use-case-id: %w", err)
		}

		payload, err = workload.ApplyUseCase(payload, id)
		if err != nil {
			return nil, err
		}
	}

	return payload, nil
}

// placementMode names the Enclave placement the flags ask for: "manual" for
// --enclave, "none" otherwise. --use-case-id alone asks for no placement. It
// reads the flags only; a spec can still choose its own placement.
func placementMode(cmd *cobra.Command) string {
	if cmd.Flags().Changed("enclave") {
		return workload.EnclaveSelectionPolicyManual
	}

	return "none"
}
