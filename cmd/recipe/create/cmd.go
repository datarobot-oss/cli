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

	"github.com/datarobot/cli/cmd/recipe/internal/filearg"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat   outputformat.OutputFormat
		fromFile       string
		name           string
		description    string
		descriptorFile string
		format         string
	)

	cmd := &cobra.Command{
		Use:   "create [<file>]",
		Short: "Register a recipe from a file.",
		Long: `Register a recipe from a JSON or YAML file.

The file is either a full create request (name, description, descriptor,
payload) or a bare payload (a top-level "format"), which then needs --name.
Flags override values in the file. The payload is one of:

  dynamo.dgd/v1beta1   "objects": a DynamoGraphDeployment and its ConfigMaps
  vllm.recipe/v1       "recipe": a recipes.vllm.ai model document

Content that fails policy is still created, with status invalid and the
reasons listed; the command then exits non-zero.

Example:
  dr recipe create qwen3-32b.vllm.yaml
  dr recipe create qwen3-0.6b.dgd.yaml --name qwen3-0-6b-agg
  dr recipe create --from-file recipe.json --description "Kimi on 16 B200"`,
		Args:         cobra.MaximumNArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			path, err := filearg.Resolve(args, fromFile, true)
			if err != nil {
				return err
			}

			file, err := llm.ReadRecipeFile(path)
			if err != nil {
				return err
			}

			format = llm.PayloadFormat(file.Payload)

			req, err := buildRequest(cmd, file, name, description, descriptorFile)
			if err != nil {
				return err
			}

			r, err := llm.CreateRecipe(req)
			if err != nil {
				return err
			}

			if err := llm.RenderRecipe(cmd.OutOrStdout(), outputFormat, *r); err != nil {
				return err
			}

			if r.Status == llm.StatusInvalid {
				return fmt.Errorf("recipe %s was created but is invalid and cannot be deployed: fix the content and run 'dr recipe patch %s <file> --if-version %d'",
					r.ID, r.ID, r.Version)
			}

			return nil
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	filearg.Add(cmd, &fromFile)
	cmd.Flags().StringVar(&name, "name", "", "Recipe name (required for a bare payload)")
	cmd.Flags().StringVar(&description, "description", "", "Recipe description")
	cmd.Flags().StringVar(&descriptorFile, "descriptor-file", "", "JSON or YAML descriptor; inferred from the payload when omitted")

	telemetry.TrackWith(cmd, func(c *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"format":          format,
			"name_flag":       c.Flags().Changed("name"),
			"descriptor_file": descriptorFile != "",
			"output_format":   string(outputFormat),
		}
	})

	return cmd
}

func buildRequest(cmd *cobra.Command, file *llm.RecipeFile, name, description, descriptorFile string) (llm.CreateRequest, error) {
	req := llm.CreateRequest{
		Name:        file.Name,
		Description: file.Description,
		Descriptor:  file.Descriptor,
		Payload:     file.Payload,
	}

	if cmd.Flags().Changed("name") {
		req.Name = name
	}

	if cmd.Flags().Changed("description") {
		req.Description = description
	}

	if descriptorFile != "" {
		d, err := workload.ReadSpecFile(descriptorFile)
		if err != nil {
			return req, err
		}

		req.Descriptor = d
	}

	if req.Name == "" {
		return req, errors.New("the recipe needs a name: add \"name\" to the file or pass --name")
	}

	return req, nil
}
