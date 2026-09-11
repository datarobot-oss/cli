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

package register

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/enclave"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var (
		name   string
		labels []string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new enclave.",
		Long: `Register a new enclave (outpost) with your DataRobot tenant.

Registration returns the enclave's ID and status along with its one-shot
installation secrets — the credentials the installer materializes as
Kubernetes Secrets on the enclave cluster. Those secrets are shown only once
and are never persisted server-side, so capture them from this command's
output (use --output-format json to save them programmatically).

After the installer applies the secrets and reports in, the enclave activates
on its own; there is no separate activate command.

Example:
  dr enclave register --name my-enclave
  dr enclave register --name my-enclave --label team=research --label env=prod
  dr enclave register --name my-enclave --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			parsedLabels, err := enclave.ParseLabels(labels)
			if err != nil {
				return err
			}

			result, err := enclave.RegisterEnclave(name, parsedLabels)
			if err != nil {
				return err
			}

			return enclave.RenderRegistration(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	cmd.Flags().StringVar(&name, "name", "", "Name for the new enclave (required)")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringArrayVar(&labels, "label", nil,
		"Label to attach as key=value (repeatable)")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"label_count":   len(labels),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
