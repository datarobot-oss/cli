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

package logs

import (
	"fmt"
	"strings"

	"github.com/datarobot/cli/cmd/artifact/build/internal/buildargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

var validLevels = map[string]bool{
	"debug":   true,
	"info":    true,
	"warn":    true,
	"warning": true,
	"error":   true,
}

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		level        string
	)

	cmd := &cobra.Command{
		Use:   "logs [<artifact-id>] <build-id>",
		Short: "Fetch build logs.",
		Long: `Fetch the structured log stream for a build.

When invoked with one positional argument the artifact-id is read from
.wapi/config.json in the current directory. When invoked with two, the
first argument is the artifact-id and the second is the build-id.

The server emits one structured JSON record per line; the default
output drops records below INFO. Use --level debug to keep everything.

JSON output emits a single array document so the result can be piped
to jq directly.

Examples:
  dr artifact build logs b-xyz-456
  dr artifact build logs art-abc-123 b-xyz-456
  dr artifact build logs art-abc-123 b-xyz-456 --level debug
  dr artifact build logs art-abc-123 b-xyz-456 --output-format json`,
		Args:         cobra.RangeArgs(1, 2),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			lower := strings.ToLower(level)
			if !validLevels[lower] {
				return fmt.Errorf("invalid --level %q: use debug, info, warn, or error", level)
			}

			artifactID, buildID, err := buildargs.ResolvePositional(args)
			if err != nil {
				return err
			}

			entries, err := workload.GetArtifactBuildLogs(artifactID, buildID)
			if err != nil {
				return err
			}

			entries = workload.FilterLogsByLevel(entries, lower)

			return workload.RenderBuildLogs(outputFormat, entries)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&level, "level", "info", "Minimum log level to show (debug, info, warn, error).")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		artifactID, buildID, _ := buildargs.ResolvePositional(args)

		return map[string]any{
			"artifact_id":   artifactID,
			"build_id":      buildID,
			"level":         level,
			"output_format": string(outputformat.GetFormat(cmd)),
		}
	})

	return cmd
}
