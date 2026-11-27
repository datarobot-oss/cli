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
		opts         llm.ListOptions
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the tenant's recipes.",
		Long: `List the tenant's recipes, newest first, without their payloads.

Filters map one-to-one to the API's: --model, --kind, --backend and --gpu-sku
filter on the content, --status defaults to any (so a recipe that failed
validation still shows), and --search matches name and description.

Example:
  dr recipe list
  dr recipe list --status available --gpu-sku B200
  dr recipe list --model moonshotai/Kimi-K3.1-Pro --kind dynamo
  dr recipe list --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			if err := checkEnum("kind", opts.Kind, "dynamo", "vllm"); err != nil {
				return err
			}

			if err := checkEnum("status", opts.Status, "any", "available", "invalid"); err != nil {
				return err
			}

			recipes, err := llm.ListRecipes(opts)
			if err != nil {
				return err
			}

			return llm.RenderRecipes(cmd.OutOrStdout(), outputFormat, recipes)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	f := cmd.Flags()
	f.StringVar(&opts.Search, "search", "", "Case-insensitive match on name and description")
	f.StringVar(&opts.Model, "model", "", "Only recipes for this model repository id")
	f.StringVar(&opts.Kind, "kind", "", "Only recipes of this kind: dynamo or vllm")
	f.StringVar(&opts.Backend, "backend", "", "Only recipes for this inference backend")
	f.StringVar(&opts.GPUSku, "gpu-sku", "", "Only recipes for this GPU SKU, e.g. H100 or B200")
	f.StringVar(&opts.Status, "status", "", "any (default), available or invalid")
	f.StringVar(&opts.CreatedBy, "created-by", "", "Only recipes created by this user id")
	f.StringSliceVar(&opts.IDs, "id", nil, "Only these recipe ids (repeatable)")
	f.Var(countflags.PositiveInt(&opts.Limit, 50), "limit", "Maximum number of recipes to return")
	f.Var(countflags.NonNegativeInt(&opts.Offset, 0), "offset", "Number of recipes to skip")

	telemetry.TrackWith(cmd, func(c *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"kind":          opts.Kind,
			"status":        opts.Status,
			"filtered":      c.Flags().NFlag() > 0,
			"limit":         opts.Limit,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func checkEnum(flag, value string, allowed ...string) error {
	if value == "" {
		return nil
	}

	for _, a := range allowed {
		if value == a {
			return nil
		}
	}

	return fmt.Errorf("invalid --%s %q: must be one of %v", flag, value, allowed)
}
