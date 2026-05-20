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

package create

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/environment/envutil"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		name         string
		description  string
		rawPackages  []string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pipeline execution environment",
		Long: `Create a new pipeline execution environment.

A new environment is registered with an initial version (v1) containing
the supplied pip packages. The environment may be referenced by
pipelines once its first version reaches the READY state.

Example:
  dr pipelines environment create --name ml-base --package numpy --package pandas
  dr pipelines environment create --name ml-base --packages numpy,pandas==2.0 --description "training base" --output json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if outputFormat != "" && outputFormat != "json" {
				return fmt.Errorf("invalid output format: %s (supported: json)", outputFormat)
			}

			if name == "" {
				return errors.New("--name is required")
			}

			packages, err := envutil.NormalizePackages(rawPackages)
			if err != nil {
				return err
			}

			result, err := pipeline.CreateEnvironment(name, description, packages)
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

	cmd.Flags().StringVar(&name, "name", "", "Environment name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	cmd.Flags().StringSliceVar(&rawPackages, "package", nil, "Pip package spec (repeatable, also accepts comma-separated values)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format (json)")

	return cmd
}
