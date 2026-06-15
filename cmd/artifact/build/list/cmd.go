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
	"fmt"

	"github.com/datarobot/cli/cmd/artifact/build/internal/buildargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat workload.OutputFormat
		limit        int
	)

	cmd := &cobra.Command{
		Use:   "list [<artifact-id>]",
		Short: "List builds for an artifact.",
		Long: `List builds in reverse-chronological order for an artifact.

The artifact-id argument is optional when run inside a directory linked
via 'dr artifact code init'.

Examples:
  dr artifact build list
  dr artifact build list art-abc-123 --limit 10
  dr artifact build list art-abc-123 --output-format json`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("invalid --limit %d: must be positive", limit)
			}

			artifactID, err := buildargs.ResolveOptional(args)
			if err != nil {
				return err
			}

			builds, err := workload.ListArtifactBuilds(artifactID, limit)
			if err != nil {
				return err
			}

			return workload.RenderBuilds(outputFormat, builds)
		},
	}

	workload.AddOutputFlag(cmd, &outputFormat)
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of builds to return.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"artifact_id":   telemetry.FirstArg(args),
			"limit":         limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
