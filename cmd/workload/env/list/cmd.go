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

package list

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:   "list <workload-id>",
		Short: "List a workload's environment variables.",
		Long: `List the environment variables on the artifact a workload is currently
running. Only the primary container's variables are shown; artifacts with
additional (sidecar) containers are not yet supported by this command.

Plain variables print as "NAME=VALUE". Credential-backed variables print as
"NAME (dr-credential:<credential-id>/<credential-key>)" instead -- their
secret value is never resolved or printed; it only ever exists inside the
stored credential, not in the artifact spec.

By default, output is human-readable (one line per variable). Use
--output-format json for the full array, including credential references.

Example:
  dr workload env list 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload env list 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			_, artifact, err := workload.ResolveWorkloadArtifact(args[0])
			if err != nil {
				return err
			}

			return workload.RenderEnvironmentVars(outputFormat, workload.PrimaryEnvironmentVars(*artifact))
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
