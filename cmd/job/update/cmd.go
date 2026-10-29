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
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var runtimeFile string

	cmd := &cobra.Command{
		Use:   "update <job-id>",
		Short: "Change a job's name, description, artifact or runtime.",
		Long: `Change what a job's future runs use. Runs already started keep what they started with.

--artifact-id points the job at another job artifact, such as a newly locked
version. --runtime-file replaces the runtime as a whole; if it leaves out
enclaveSelectionPolicy and enclaves, the job keeps its Enclave placement.

Example:
  dr job update 68b0c1d2e3f4a5b6c7d8e9f0 --artifact-id 68b0c1d2e3f4a5b6c7d8e9f1
  dr job update 68b0c1d2e3f4a5b6c7d8e9f0 --runtime-file runtime.yaml
  dr job update 68b0c1d2e3f4a5b6c7d8e9f0 --name train-v2 --description "Nightly training"`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			changes, err := jobargs.Body(cmd, "", fields)
			if err != nil {
				return err
			}

			if runtimeFile != "" {
				changes["runtime"], err = jobargs.RuntimeFromFile(runtimeFile)
				if err != nil {
					return err
				}
			}

			if len(changes) == 0 {
				return errors.New("nothing to update: pass --name, --description, --artifact-id or --runtime-file")
			}

			updated, err := job.UpdateJob(args[0], changes)
			if err != nil {
				return err
			}

			return job.RenderJob(outputFormat, *updated)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().String("name", "", "New job name")
	cmd.Flags().String("description", "", "New job description")
	cmd.Flags().String("artifact-id", "", "The job artifact future runs execute")
	cmd.Flags().StringVar(&runtimeFile, "runtime-file", "", "Path to a JSON or YAML runtime block that replaces the stored one")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"output_format":        string(outputFormat),
			"artifact_id_provided": cmd.Flags().Changed("artifact-id"),
			"runtime_provided":     runtimeFile != "",
		}
	})

	return cmd
}
