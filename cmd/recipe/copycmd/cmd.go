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

// Package copycmd implements `dr recipe copy`. The directory is not named
// `copy` because that would shadow Go's built-in copy() in this package.
package copycmd

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		name         string
		description  string
		version      int
	)

	cmd := &cobra.Command{
		Use:   "copy <recipe-id> --name <new-name>",
		Short: "Create a new recipe from an existing one.",
		Long: `Create a new recipe with another recipe's payload and descriptor.

The API has no copy endpoint: the CLI reads the source (--version reads a past
version) and creates an independent recipe, owned by you, under the new name.
It is how you customise a recipe you cannot patch. Validation runs on the copy
as on any create.

Example:
  dr recipe copy 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-k3-custom
  dr recipe copy 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-k3-v2 --version 2`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			src, err := llm.GetRecipe(args[0], version)
			if err != nil {
				return err
			}

			req := llm.CreateRequest{
				Name:        name,
				Description: src.Description,
				Descriptor:  src.Descriptor,
				Payload:     src.Payload,
			}

			if cmd.Flags().Changed("description") {
				req.Description = description
			}

			if len(req.Payload) == 0 {
				return fmt.Errorf("recipe %s returned no payload to copy", src.ID)
			}

			r, err := llm.CreateRecipe(req)
			if err != nil {
				return err
			}

			if err := llm.RenderRecipe(cmd.OutOrStdout(), outputFormat, *r); err != nil {
				return err
			}

			if r.Status == llm.StatusInvalid {
				return fmt.Errorf("copy %s was created but is invalid and cannot be deployed", r.ID)
			}

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().StringVar(&name, "name", "", "Name of the new recipe (required)")
	cmd.Flags().StringVar(&description, "description", "", "Description of the new recipe (default: the source's)")
	cmd.Flags().Var(countflags.NonNegativeInt(&version, 0), "version", "Copy a past version of the source (default: latest)")
	_ = cmd.MarkFlagRequired("name")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"version_given": version > 0,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
