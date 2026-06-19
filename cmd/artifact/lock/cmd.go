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

package lock

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
		Use:   "lock <artifact-id>",
		Short: "Lock a draft artifact",
		Long: `Promote a draft artifact to locked and show the result.

Locking is one-way: a locked artifact's name, description, and spec become
immutable, it gets a version number, and it can never be deleted or
unlocked. Locking an already locked artifact is rejected.

Before locking, the server validates build completeness: every container
built from source (imageBuildConfig) must have its code uploaded and an
image build completed; otherwise the lock is rejected with an error naming
what is missing.

By default, output is human-readable. Use --output-format json for machine-parseable output.

Example:
  dr artifact lock 68b0c1d2e3f4a5b6c7d8e9f0
  dr artifact lock 68b0c1d2e3f4a5b6c7d8e9f0 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			artifact, err := workload.LockArtifact(args[0])
			if err != nil {
				return err
			}

			return workload.RenderArtifact(outputFormat, *artifact)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
