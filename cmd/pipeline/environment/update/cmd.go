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
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/environment/envutil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		rawPackages  []string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "update <environment-id>",
		Short: "Add a new version to a pipeline execution environment",
		Long: `Update a pipeline execution environment by appending packages.

Updating creates a new immutable version of the environment containing
the supplied pip packages. Existing versions are unchanged.

Example:
  dr pipelines environment update env-123 --package scikit-learn
  dr pipelines environment update env-123 --package "scikit-learn==1.5,torch" --output json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			packages, err := envutil.NormalizePackages(rawPackages)
			if err != nil {
				return err
			}

			result, err := pipeline.UpdateEnvironment(args[0], packages)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return envutil.PrintEnvironmentJSON(*result)
			}

			envutil.PrintEnvironmentHuman(*result)

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}
