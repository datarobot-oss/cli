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

// Package doctor implements `dr doctor`: checks on what the install says about
// itself, so a caller learns before a deploy whether the install will accept it.
package doctor

import (
	"github.com/spf13/cobra"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/features"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/platform"
	platformdoctor "github.com/datarobot/cli/internal/platform/doctor"
	"github.com/datarobot/cli/internal/telemetry"
)

const (
	gate  = "platform-doctor"
	title = "Platform doctor report"
)

var describeFn = platform.Describe

// Cmd returns the `dr doctor` command.
func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "doctor",
		GroupID: "core",
		Short:   "🩺 Check whether the install accepts your deploys",
		Long: `Check what the install says about itself against what a deploy needs.

The doctor reads the same report as 'dr info', with your own credentials, and
judges it. Each check is OK, WARN, FAIL or SKIP, and every finding that needs
action names the remedy and who acts on it. It is read-only: nothing is
written to the install or to disk.

A release without the Workload API is a WARN, because a Custom Application
still deploys. A refusal from the Workload API is a FAIL that names the seat or
the feature flag behind it, except a probe that was throttled or could not be
read, which is a WARN to run again. A check that depends on an earlier failure
is a SKIP, so an unreachable install fails once.

Exit code is 0 when no check FAILs (warnings are allowed) and 1 when at least
one check FAILs. Pass --output-format json for a machine-readable report.`,
		Example: `  dr doctor
  dr doctor --output-format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			useEnvCredentials()

			results := core.NewRunner(platformdoctor.Checks(describeFn)...).Run(cmd.Context())
			report := core.NewTitledReport(title, results)

			if err := render(cmd, outputFormat, report); err != nil {
				return err
			}

			if report.ExitCode() == 1 {
				cmd.SilenceErrors = true

				return cli.ErrSilent
			}

			return nil
		},
	}

	features.SetGate(cmd, gate)

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{"output_format": string(outputFormat)}
	})

	return cmd
}

// useEnvCredentials makes a complete DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN
// pair win over the active profile, as it does for the other commands. It neither
// verifies the pair nor starts a login: a rejected pair is a FAIL of
// platform.install.read. Anything less than a complete pair leaves the profile.
func useEnvCredentials() {
	creds := auth.GetEnvCredentials()
	if creds.Endpoint == "" || creds.Token == "" {
		return
	}

	_ = viperx.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
	_ = viperx.BindEnv("token", "DATAROBOT_API_TOKEN")
}

// render writes the report in the requested format.
func render(cmd *cobra.Command, format outputformat.OutputFormat, report core.Report) error {
	if format == outputformat.OutputFormatJSON {
		return core.WriteJSON(cmd.OutOrStdout(), report)
	}

	return core.WriteText(cmd.OutOrStdout(), report)
}
