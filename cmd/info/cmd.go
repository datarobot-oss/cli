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

// Package info implements `dr info`: what the install says about itself that
// decides whether a Custom Application deploy will work.
package info

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/features"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/platform"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// describeFn is the read behind the command, a seam so the shell can be
// tested without an install to ask.
var describeFn = platform.Describe

// gate is the feature gate name. It is independent of the command name, so the
// command can be renamed without silently hiding it.
const gate = "platform-info"

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "info",
		GroupID: "core",
		Short:   "🧭 Report what the DataRobot install supports",
		Long: `Report what the install says about itself, from public routes only.

Which execution environments and resource bundles a Custom Application can
use, which feature flags are on, the caller's seat licenses, and the install's
release and URL differ per install, so a pipeline copied from another install
breaks. This command reads them in one call, with the caller's own credentials.

The report states facts and leaves the verdict to the reader. A source the
caller cannot read becomes an "unavailable" section that carries the route's
reason, and the other sections still report. The command exits zero for that.
It exits non-zero only when it cannot reach the install or authenticate.

Seat licenses map each license to whether the caller has it. A license that is
absent is not enforced on that install, so an empty map does not mean blocked.

JSON output puts the report under "platform", beside "schemaVersion", and is
described by docs/schemas/platform-info.schema.json.

Example:
  dr info
  dr info --output-format json | jq '.platform.sections.executionEnvironments.data.items'`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			report, err := describeFn(cmd.Context())
			if err != nil {
				return err
			}

			return platform.RenderTo(cmd.OutOrStdout(), outputFormat, *report)
		},
	}

	features.SetGate(cmd, gate)

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat)}
	})

	return cmd
}
