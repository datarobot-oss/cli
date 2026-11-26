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

package rollout

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

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
		outputFormat  outputformat.OutputFormat
		recipeID      string
		recipeVersion int
		latest        bool
		dryRun        bool
		noSmoke       bool
		poll          pollflags.Set
	)

	cmd := &cobra.Command{
		Use:   "rollout <workload-id>",
		Short: "Roll a running LLM workload onto another recipe version.",
		Long: `Move a running llm workload onto another recipe version, or another recipe,
with a rolling replacement on the same artifact: the new generation starts next
to the old one and traffic moves when it is ready, so the endpoint and served
name do not change.

A recipe change is a replacement on the server (a settings update refuses
it), and deploying again would create a new workload and endpoint.

  --recipe-version N   pin version N of the workload's recipe
  --latest             unpin: resolve the latest version when scheduled
  --recipe <id>        switch to another recipe (with either of the above)

--wait polls until the replacement finishes and then sends one chat request;
--no-smoke skips that request.

Example:
  dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --recipe-version 3 --wait
  dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --latest
  dr llm rollout 68b0c1d2e3f4a5b6c7d8e9f0 --recipe 6a1f0c2e9b7d4e3f2a1b0c9d --recipe-version 1`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			if recipeVersion > 0 && latest {
				return errors.New("--recipe-version and --latest are mutually exclusive")
			}

			if recipeVersion == 0 && !latest && recipeID == "" {
				return errors.New("say where to roll: --recipe-version N, --latest, or --recipe <id>")
			}

			plan, err := planRollout(args[0], recipeID, recipeVersion)
			if err != nil {
				return err
			}

			if dryRun {
				return printPlan(cmd, plan)
			}

			if err := workload.RefuseActiveReplacement(plan.workloadID); err != nil {
				return err
			}

			started, err := workload.StartReplacement(plan.workloadID, plan.artifactID, plan.runtime)
			if err != nil {
				return err
			}

			return report(cmd, outputFormat, plan, started, poll, noSmoke)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().StringVar(&recipeID, "recipe", "", "Switch to this recipe (default: the workload's current one)")
	cmd.Flags().Var(countflags.NonNegativeInt(&recipeVersion, 0), "recipe-version", "Pin this recipe version")
	cmd.Flags().BoolVar(&latest, "latest", false, "Unpin: resolve the latest recipe version when scheduled")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the replacement request and exit")
	cmd.Flags().BoolVar(&noSmoke, "no-smoke", false, "With --wait, skip the chat request at the end")
	pollflags.RegisterWithDefaults(cmd, &poll, pollflags.DefaultPollInterval, pollflags.DefaultPollTimeout,
		"Wait until the replacement finishes and the model answers.")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"switch_recipe":  recipeID != "",
			"version_pinned": recipeVersion > 0,
			"latest":         latest,
			"wait":           poll.Wait,
			"dry_run":        dryRun,
			"output_format":  string(outputFormat),
		}
	})

	return cmd
}

type plan struct {
	workloadID  string
	artifactID  string
	fromRecipe  string
	fromVersion string
	toRecipe    string
	toVersion   int
	runtime     json.RawMessage
}

func planRollout(workloadID, recipeID string, recipeVersion int) (*plan, error) {
	doc, err := llm.GetWorkloadDocument(workloadID)
	if err != nil {
		return nil, err
	}

	if t, _ := doc["type"].(string); t != "" && t != "llm" {
		return nil, fmt.Errorf("workload %s is a %q workload, not an llm one", workloadID, t)
	}

	artifactID, _ := doc["artifactId"].(string)
	if artifactID == "" {
		return nil, fmt.Errorf("workload %s reports no artifact", workloadID)
	}

	runtime, _ := doc["runtime"].(map[string]any)
	from, fromVersion := currentRecipe(runtime)

	to := recipeID
	if to == "" {
		to = from
	}

	if to == "" {
		return nil, fmt.Errorf("workload %s runs no recipe: pass --recipe <id>", workloadID)
	}

	target, err := llm.GetRecipe(to, recipeVersion)
	if err != nil {
		return nil, err
	}

	if target.Status == llm.StatusInvalid {
		return nil, fmt.Errorf("recipe %s (version %d) is invalid and cannot be rolled out", target.ID, target.Version)
	}

	body, err := llm.RolloutRuntime(runtime, to, recipeVersion)
	if err != nil {
		return nil, err
	}

	return &plan{
		workloadID: workloadID, artifactID: artifactID,
		fromRecipe: from, fromVersion: fromVersion,
		toRecipe: to, toVersion: recipeVersion, runtime: body,
	}, nil
}

// currentRecipe is the recipe the workload runs: the resolved one, else the
// one it asked for, with its version (resolved, pinned, or "latest").
func currentRecipe(runtime map[string]any) (string, string) {
	id, v := llm.RecipeOf(runtime, true)
	if id == "" {
		return "", ""
	}

	if v > 0 {
		return id, strconv.Itoa(v)
	}

	return id, "latest"
}

func (p *plan) target() string {
	if p.toVersion > 0 {
		return fmt.Sprintf("%s version %d", p.toRecipe, p.toVersion)
	}

	return p.toRecipe + " (latest when scheduled)"
}

func printPlan(cmd *cobra.Command, p *plan) error {
	body := map[string]any{"artifactId": p.artifactID, "strategy": "rolling", "runtime": p.runtime}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")

	return enc.Encode(body)
}

func report(cmd *cobra.Command, format outputformat.OutputFormat, p *plan, started *workload.Replacement, poll pollflags.Set, noSmoke bool) error {
	out := cmd.OutOrStdout()

	if format != outputformat.OutputFormatJSON {
		fmt.Fprintf(out, "Rolling workload %s from recipe %s (%s) to %s.\n", p.workloadID, p.fromRecipe, p.fromVersion, p.target())
	}

	final := started

	if poll.Wait {
		var err error

		final, err = waitAndSmoke(cmd, format, p, started, poll, noSmoke)
		if err != nil {
			return err
		}
	}

	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(out, "replacement", final)
	}

	fmt.Fprintf(out, "Replacement: %s\n", final.Status)

	if !poll.Wait {
		fmt.Fprintf(out, "\nFollow it with: dr llm watch %s\n", p.workloadID)
	}

	return nil
}

// waitAndSmoke waits for the replacement and confirms it: a replacement whose
// record vanished before a final status is not taken as success until the
// workload itself names the requested recipe; only then is it reported
// completed and smoke-tested.
func waitAndSmoke(cmd *cobra.Command, format outputformat.OutputFormat, p *plan, started *workload.Replacement, poll pollflags.Set, noSmoke bool) (*workload.Replacement, error) {
	final, err := workload.WaitForReplacement(cmd.Context(), p.workloadID, started, poll.Interval, poll.Timeout, nil)
	if err != nil {
		return final, err
	}

	if !workload.IsTerminalReplacementStatus(final.Status) {
		if err := confirmRecipe(p); err != nil {
			return final, err
		}

		final.Status = workload.ReplacementStatusCompleted
	}

	if noSmoke {
		return final, nil
	}

	res, err := llm.Smoke(cmd.Context(), p.workloadID)
	if err != nil {
		return final, err
	}

	if format == outputformat.OutputFormatJSON {
		return final, nil
	}

	if res.Skipped != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Smoke check skipped: "+res.Skipped)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Smoke check: the model answered a chat request.")
	}

	return final, nil
}

// confirmRecipe checks the workload now asks for the recipe and version the
// rollout requested, for a replacement that settled without a final status.
func confirmRecipe(p *plan) error {
	doc, err := llm.GetWorkloadDocument(p.workloadID)
	if err != nil {
		return fmt.Errorf("the rollout's outcome is unconfirmed: the replacement record is gone and the workload could not be read: %w", err)
	}

	runtime, _ := doc["runtime"].(map[string]any)

	id, v := llm.RecipeOf(runtime, false)
	if id != p.toRecipe || v != p.toVersion {
		return fmt.Errorf("the rollout's outcome is unconfirmed: the replacement record is gone and workload %s still names recipe %s version %d; check 'dr llm status %s' and 'dr workload events %s'",
			p.workloadID, id, v, p.workloadID, p.workloadID)
	}

	return nil
}
