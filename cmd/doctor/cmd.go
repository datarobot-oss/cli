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

// Package doctor implements `dr doctor`.
package doctor

import (
	"context"
	"errors"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/features"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/platform"
	"github.com/datarobot/cli/internal/platform/doctor"
	"github.com/datarobot/cli/internal/telemetry"
)

const (
	gate  = "platform-doctor"
	title = "Platform doctor report"
)

var describeFn = describeInstall

// describeInstall names a missing or malformed endpoint, which the request
// would report as "Empty URL.".
func describeInstall(ctx context.Context) (*platform.Report, error) {
	if config.GetBaseURL() == "" {
		return nil, errors.New("no valid endpoint is configured")
	}

	return platform.Describe(ctx)
}

// installURL returns scheme://host of the configured endpoint, without any
// credentials in it, or "" when there is none.
func installURL() string {
	parsed, err := url.Parse(config.GetBaseURL())
	if err != nil || parsed.Host == "" {
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host
}

// Cmd returns the `dr doctor` command.
func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "doctor",
		GroupID: "core",
		Short:   "🩺 Check that the install accepts your deploys",
		Long: `Judge what the install says about itself, as 'dr info' reports it, against
what a deploy needs.

Each check is OK, WARN, FAIL or SKIP. A finding that needs action names the
remedy and who acts. The command is read-only.

Exit code is 0 when no check FAILs and 1 when at least one does. Pass
--output-format json for a machine-readable report.`,
		Example: `  dr doctor
  dr doctor --output-format json`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			auth.BindEnvCredentials()

			heading := title
			if endpoint := installURL(); endpoint != "" {
				heading += " for " + endpoint
			}

			results := core.NewRunner(platformdoctor.Checks(describeFn)...).Run(cmd.Context())
			report := core.NewTitledReport(heading, results)

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

// render writes the report in the requested format.
func render(cmd *cobra.Command, format outputformat.OutputFormat, report core.Report) error {
	if format == outputformat.OutputFormatJSON {
		return core.WriteJSON(cmd.OutOrStdout(), report)
	}

	return core.WriteText(cmd.OutOrStdout(), report)
}
