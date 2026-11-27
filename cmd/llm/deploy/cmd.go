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

package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/countflags"
	"github.com/datarobot/cli/internal/llm"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		opts         llm.DeployOptions
		toolCalling  string
		reasoning    string
		useCaseID    string
		dryRun       bool
		noSmoke      bool
		poll         pollflags.Set
	)

	cmd := &cobra.Command{
		Use:   "deploy --recipe <recipe-id> --name <name> --bundle <bundle-id>",
		Short: "Deploy a recipe as an LLM workload.",
		Long: `Deploy a recipe as an llm workload, in one request: an llm artifact for the
model (or --artifact-id to reuse one) and a runtime naming the recipe.

The model defaults to the recipe's own and the served name (what clients send
as "model") to its repository name. The recipe must be available: one that
failed validation is refused here. The server also refuses a recipe that is
not this tenant's, not for the cluster's engine, or not serving the model.

--wait polls until the workload is running and then sends one chat request,
so done means the model answers; --no-smoke skips that request.

Model weights are stored on the cluster's default model weight storage unless
--model-weight-size creates a storage for this workload (deleted with it) or
--model-weight-storage-id names an existing one. A workload placed on an
Enclave (--use-case-id with a Use Case linked to one) needs one of them: an
Enclave has no default.

Example:
  dr llm deploy --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-prod --bundle gpu.b200.x8 --wait
  dr llm deploy --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --name qwen --bundle gpu.h100.x2 \
    --model Qwen/Qwen3-32B --served-name qwen3-32b --recipe-version 2
  dr llm deploy --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --name kimi-prod --bundle gpu.b200.x8 --dry-run
  dr llm deploy --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --name qwen-enclave --bundle gpu.h100.x1 \
    --use-case-id 68a1b2c3d4e5f6a7b8c9d0e1 --model-weight-size 50Gi --wait`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)
			opts.ToolCalling = llm.Capability(toolCalling)
			opts.Reasoning = llm.Capability(reasoning)

			body, recipe, err := buildBody(cmd, &opts, useCaseID)
			if err != nil {
				return err
			}

			if dryRun {
				return printJSON(cmd.OutOrStdout(), body)
			}

			created, err := llm.Deploy(body)
			if err != nil {
				return err
			}

			return report(cmd, outputFormat, created, recipe, poll, noSmoke)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)

	f := cmd.Flags()
	f.StringVar(&opts.RecipeID, "recipe", "", "Recipe to deploy with (required)")
	f.Var(countflags.NonNegativeInt(&opts.RecipeVersion, 0), "recipe-version", "Pin a recipe version (default: the latest when scheduled)")
	f.StringVar(&opts.Name, "name", "", "Workload name (required)")
	f.StringVar(&opts.Description, "description", "", "Workload description")
	f.StringSliceVar(&opts.Bundles, "bundle", nil, "GPU resource bundle for the workers, in preference order (repeatable, required)")
	f.StringVar(&opts.Model, "model", "", "Model repository id (default: the recipe's model)")
	f.StringVar(&opts.Revision, "revision", "", "Model revision")
	f.StringVar(&opts.ModelSource, "model-source", llm.SourceHuggingFace, "Where the weights come from: huggingFace or s3")
	f.StringVar(&opts.ServedName, "served-name", "", "Name clients send as \"model\" (default: from the repository name)")
	f.StringVar(&toolCalling, "tool-calling", "auto", "Tool calling: auto, on or off")
	f.StringVar(&reasoning, "reasoning", "auto", "Reasoning output: auto, on or off")
	f.StringVar(&opts.ArtifactID, "artifact-id", "", "Reuse an existing llm artifact instead of creating one")
	f.BoolVar(&opts.TokenGrid, "token-grid", false, "Opt the workload into Token Grid")
	f.Var(countflags.NonNegativeInt(&opts.ConcurrencySlots, 0), "concurrency-slots", "Token Grid concurrent requests")
	f.Var(countflags.NonNegativeInt(&opts.TokensPerSecond, 0), "tokens-per-second", "Token Grid throughput (server default 500)")
	f.StringVar(&opts.ModelWeightSize, "model-weight-size", "", "Create a model weight storage of this size for the workload, deleted with it (e.g. 50Gi)")
	f.StringVar(&opts.ModelWeightStorageID, "model-weight-storage-id", "", "Serve the weights from an existing model weight storage")
	f.StringVar(&useCaseID, "use-case-id", "", "Link the workload to a Use Case (one linked to an Enclave places it there)")
	f.BoolVar(&dryRun, "dry-run", false, "Print the request and exit")
	f.BoolVar(&noSmoke, "no-smoke", false, "With --wait, skip the chat request at the end")
	pollflags.RegisterWithDefaults(cmd, &poll, pollflags.DefaultPollInterval, pollflags.DefaultPollTimeout,
		"Wait until the workload is running and the model answers.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"recipe_version_pinned": opts.RecipeVersion > 0,
			"reuse_artifact":        opts.ArtifactID != "",
			"model_source":          opts.ModelSource,
			"token_grid":            opts.TokenGrid,
			"model_weight":          modelWeightKind(opts),
			"wait":                  poll.Wait,
			"dry_run":               dryRun,
			"output_format":         string(outputFormat),
		}
	})

	return cmd
}

// buildBody reads and checks the recipe, defaults the model to the recipe's,
// and builds the POST /workloads/ body.
func buildBody(cmd *cobra.Command, opts *llm.DeployOptions, useCaseID string) (map[string]any, *llm.Recipe, error) {
	if err := checkFlags(opts); err != nil {
		return nil, nil, err
	}

	recipe, err := llm.GetRecipe(opts.RecipeID, opts.RecipeVersion)
	if err != nil {
		return nil, nil, err
	}

	if recipe.Status == llm.StatusInvalid {
		llm.RenderDiagnostics(cmd.ErrOrStderr(), *recipe)

		return nil, nil, fmt.Errorf("recipe %s (version %d) is invalid and cannot be deployed", recipe.ID, recipe.Version)
	}

	if opts.ArtifactID == "" && opts.Model == "" {
		opts.Model = recipe.ModelName()
	}

	body, err := llm.BuildDeployRequest(*opts)
	if err != nil {
		return nil, nil, err
	}

	if useCaseID != "" {
		body["useCaseId"] = useCaseID
	}

	return body, recipe, nil
}

// modelWeightKind is the model weight choice for telemetry, without values.
func modelWeightKind(opts llm.DeployOptions) string {
	switch {
	case opts.ModelWeightStorageID != "":
		return "storage"
	case opts.ModelWeightSize != "":
		return "inline"
	}

	return "default"
}

func checkFlags(opts *llm.DeployOptions) error {
	if opts.RecipeID == "" {
		return errors.New("--recipe is required")
	}

	if opts.ArtifactID != "" && (opts.Model != "" || opts.ServedName != "" || opts.Revision != "") {
		return errors.New("--artifact-id reuses an artifact's model: drop --model, --served-name and --revision")
	}

	return nil
}

func report(cmd *cobra.Command, format outputformat.OutputFormat, created *llm.CreatedWorkload, recipe *llm.Recipe, poll pollflags.Set, noSmoke bool) error {
	out := cmd.OutOrStdout()

	if format != outputformat.OutputFormatJSON {
		fmt.Fprintf(out, "Workload %s (%s) created from recipe %s.\n", created.ID, created.Name, recipe.ID)
		fmt.Fprintf(out, "Artifact: %s\n", created.ArtifactID)
	}

	if poll.Wait {
		wl, err := workload.WaitForWorkload(cmd.Context(), created.ID, workload.Serving{ArtifactID: created.ArtifactID},
			poll.Interval, poll.Timeout, nil)
		if err != nil {
			return err
		}

		created.Status = wl.Status
		created.Endpoint = wl.Endpoint

		if !noSmoke {
			if err := smoke(cmd, format, created.ID); err != nil {
				return err
			}
		}
	}

	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(out, "workload", created)
	}

	fmt.Fprintf(out, "Status: %s\n", created.Status)
	fmt.Fprintf(out, "\nNext:\n  dr llm status %s\n  dr llm chat %s \"Hello\"\n  dr workload logs %s\n", created.ID, created.ID, created.ID)

	return nil
}

func smoke(cmd *cobra.Command, format outputformat.OutputFormat, workloadID string) error {
	res, err := llm.Smoke(cmd.Context(), workloadID)
	if err != nil {
		return err
	}

	if format == outputformat.OutputFormatJSON {
		return nil
	}

	if res.Skipped != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Smoke check skipped: "+res.Skipped)

		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Smoke check: the model answered a chat request.")

	return nil
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}
