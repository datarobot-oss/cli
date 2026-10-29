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

	"github.com/datarobot/cli/cmd/job/internal/jobargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/job"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

var fields = []jobargs.Field{
	{Flag: "name", Key: "name"},
	{Flag: "description", Key: "description"},
	{Flag: "artifact-id", Key: "artifactId"},
	{Flag: "use-case-id", Key: "useCaseId"},
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var specFile, runtimeFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a run-to-completion job.",
		Long: `Create a job: the job artifact its runs execute and the runtime they start with.

Creating a job does not start a run; start one with 'dr job run <job-id>'.

Give the fields as flags, in a JSON or YAML spec file (the POST /jobs body,
sent with any field the server accepts), or both: a flag fills a field the
file leaves out but may not contradict one it sets.

The artifact must be a job artifact (type "job") you can use. Build one with
'dr artifact create' and the artifact code and build commands.

The runtime sets the resource bundle (for example a GPU size) and, with a use
case, where runs are placed:

  containerGroups:
    - name: default
      resourceBundles: [gpu.a100.1]
  enclaveSelectionPolicy: availability   # one of the use case's Enclaves

A run is never scaled, so replicaCount, autoscaling and scale-to-zero are
refused. Enclave placement needs --use-case-id; 'manual' with one name in
'enclaves' pins one Enclave.

Example:
  dr job create --name train --artifact-id 68b0c1d2e3f4a5b6c7d8e9f0
  dr job create --name train --artifact-id 68b0c1d2e3f4a5b6c7d8e9f0 --runtime-file runtime.yaml \
    --use-case-id 68b0aa11bb22cc33dd44ee55
  dr job create --spec-file job.yaml --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			body, err := jobargs.Body(cmd, specFile, fields)
			if err != nil {
				return err
			}

			if runtimeFile != "" {
				if _, ok := body["runtime"]; ok {
					return errors.New("--runtime-file contradicts runtime in the spec file; set it in one place")
				}

				body["runtime"], err = jobargs.RuntimeFromFile(runtimeFile)
				if err != nil {
					return err
				}
			}

			if body["name"] == nil || body["name"] == "" {
				return errors.New("a job needs a name: pass --name or set name in the spec file")
			}

			if body["artifactId"] == nil || body["artifactId"] == "" {
				return errors.New("a job needs a job artifact: pass --artifact-id or set artifactId in the spec file")
			}

			created, err := job.CreateJob(body)
			if err != nil {
				return err
			}

			return job.RenderJob(outputFormat, *created)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&specFile, "spec-file", "", "Path to a JSON or YAML POST /jobs body")
	cmd.Flags().String("name", "", "Job name (names are not unique)")
	cmd.Flags().String("description", "", "Job description")
	cmd.Flags().String("artifact-id", "", "The job artifact every run executes")
	cmd.Flags().String("use-case-id", "", "Link the job to this Use Case; needed for Enclave placement")
	cmd.Flags().StringVar(&runtimeFile, "runtime-file", "", "Path to a JSON or YAML runtime block")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format":        string(outputFormat),
			"spec_file_provided":   specFile != "",
			"runtime_provided":     runtimeFile != "",
			"use_case_id_provided": cmd.Flags().Changed("use-case-id"),
		}
	})

	return cmd
}
