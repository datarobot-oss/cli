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

// Package codesync wires the sync engine into the `dr artifact code sync`
// Cobra command. Named "codesync" (rather than "sync") to avoid colliding
// with the standard library and with internal/workload/sync; the directory
// matches so callers can import without an alias.
package codesync

import (
	"errors"
	"fmt"
	"io"

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/sync/display"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// engineRunner is the subset of *sync.Engine that the sync command
// drives. Defined here so cmd_test.go can substitute a fake without
// piling test seams onto the engine package.
type engineRunner interface {
	Plan() (*sync.SyncPlan, error)
	Execute(*sync.SyncPlan) (*sync.Result, error)
	Close() error
	StaleRollbackRestored() bool
	Fetcher() display.ContentFetcher
}

// realEngine adapts *sync.Engine to engineRunner. Only Fetcher needs an
// adapter: *sync.Engine.Fetcher returns its private *engineFetcher,
// which already implements display.ContentFetcher, but Go interface
// satisfaction is invariant over return types.
type realEngine struct{ *sync.Engine }

func (r realEngine) Fetcher() display.ContentFetcher { return r.Engine.Fetcher() }

// Deps holds the externally-injected collaborators for the sync
// command. Tests build a Deps with fakes and pass it to cmdWithDeps;
// production callers go through Cmd() which uses defaultDeps().
type Deps struct {
	NewEngine func(dir string, opts sync.Options) (engineRunner, error)
	ReadLine  func() (string, error)
}

// runFlags is the parsed view of the boolean flags that gate
// finishSync's render/prompt/execute decisions. Grouped so the inner
// helpers don't carry a three-bool tail through every signature.
type runFlags struct {
	DryRun bool
	Diff   bool
	Yes    bool
}

func defaultDeps() Deps {
	return Deps{
		NewEngine: func(dir string, opts sync.Options) (engineRunner, error) {
			e, err := sync.New(dir, opts)
			if err != nil {
				return nil, err
			}

			return realEngine{e}, nil
		},
		ReadLine: reader.ReadString,
	}
}

func init() {
	// --yes is read directly from cobra; only the env var binds to viper
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")
}

// Cmd returns the cobra.Command for `dr artifact code sync`.
func Cmd() *cobra.Command {
	return cmdWithDeps(defaultDeps())
}

func cmdWithDeps(deps Deps) *cobra.Command {
	var outputFormat workload.OutputFormat

	c := &cobra.Command{
		Use:          "sync",
		Short:        "Push and pull code changes between this directory and the linked artifact.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `Synchronize the linked DataRobot artifact with the project
directory. Computes a three-way diff against the last known state,
auto-resolves any conflicts (remote wins; your version is saved as a
*.LOCAL.<timestamp> copy), and applies the resulting plan in a single
versioned step.

Use --dry-run to preview the plan without writing anything; --diff to
also print per-file unified diffs. Both modes exit before any remote
write. --yes auto-confirms the post-plan prompt and skips any
interactive directory prompt.

Run 'dr artifact code init <artifact-id>' first to link a project
directory to an artifact.

Example:
  dr artifact code sync
  dr artifact code sync --dry-run
  dr artifact code sync --diff
  dr artifact code sync --yes
  dr artifact code sync --output-format json`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSync(cmd, outputFormat, deps)
		},
	}

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().Bool("dry-run", false, "Show plan, no writes.")
	c.Flags().Bool("diff", false, "Show plan + per-file unified diffs, no writes.")
	c.Flags().BoolP("yes", "y", false, "Skip interactive prompts; auto-confirm.")
	c.MarkFlagsMutuallyExclusive("dry-run", "diff")

	workload.AddOutputFlag(c, &outputFormat)

	telemetry.TrackWith(c, func(cmd *cobra.Command, _ []string) map[string]any {
		flags := parseRunFlags(cmd)

		return map[string]any{
			"dry_run":       flags.DryRun,
			"diff":          flags.Diff,
			"yes":           flags.Yes,
			"output_format": string(outputFormat),
		}
	})

	return c
}

func runSync(cmd *cobra.Command, outputFormat workload.OutputFormat, deps Deps) error {
	flags := parseRunFlags(cmd)

	dirFlag, _ := cmd.Flags().GetString("dir")

	dir, err := dirprompt.ResolveDir(dirFlag, flags.Yes, dirprompt.AskWithDefault)
	if err != nil {
		return err
	}

	if !wapi.Exists(dir) {
		return errors.New("not linked: run 'dr artifact code init <artifact-id>' first")
	}

	engine, err := deps.NewEngine(dir, sync.Options{DryRun: flags.DryRun, ShowDiffs: flags.Diff, Yes: flags.Yes})
	if err != nil {
		return err
	}

	defer func() {
		if cerr := engine.Close(); cerr != nil {
			log.Debug("sync engine close returned error", "err", cerr)
		}
	}()

	plan, err := engine.Plan()
	if err != nil {
		return err
	}

	if engine.StaleRollbackRestored() {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.DimStyle.Render("Recovered from interrupted sync. Working tree restored."))
	}

	return finishSync(cmd, engine, plan, outputFormat, flags, deps)
}

// parseRunFlags reads the cobra flags once and folds the
// DATAROBOT_CLI_NON_INTERACTIVE env-var override into Yes, so the
// downstream helpers see a single source of truth.
func parseRunFlags(cmd *cobra.Command) runFlags {
	yesFlag, _ := cmd.Flags().GetBool("yes")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	diff, _ := cmd.Flags().GetBool("diff")

	return runFlags{
		DryRun: dryRun,
		Diff:   diff,
		Yes:    yesFlag || viperx.GetBool("yes"),
	}
}

// finishSync handles the render → optional prompt → execute → render
// tail of the command. Pulled out so runSync's early-return paths
// (auth, lock, plan errors) stay flat.
func finishSync(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan, outputFormat workload.OutputFormat, flags runFlags, deps Deps) error {
	out := cmd.OutOrStdout()

	if outputFormat == workload.OutputFormatJSON {
		return finishJSON(engine, plan, out, flags)
	}

	if err := renderHumanPlan(cmd, engine, plan, flags.Diff); err != nil {
		return err
	}

	if flags.DryRun || flags.Diff || plan.IsEmpty() {
		return nil
	}

	if shouldPromptConflicts(plan, flags.Yes) {
		choice, err := promptConflictMenu(cmd, engine, plan, deps.ReadLine)
		if err != nil {
			return err
		}

		if choice == promptQuit {
			return nil
		}
	}

	result, err := engine.Execute(plan)
	if err != nil {
		return err
	}

	return display.PrintResult(out, result)
}

// renderHumanPlan prints the plan and optional per-file diffs.
func renderHumanPlan(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan, diffFlag bool) error {
	out := cmd.OutOrStdout()

	if err := display.PrintPlan(out, plan); err != nil {
		return err
	}

	if !diffFlag {
		return nil
	}

	return display.PrintDiffs(out, plan, engine.Fetcher())
}

// shouldPromptConflicts encapsulates the decision: prompt only when
// the user has not passed --yes and the plan actually has conflicts.
func shouldPromptConflicts(plan *sync.SyncPlan, yes bool) bool {
	return !yes && plan.HasConflicts()
}

// finishJSON is the --output-format=json analogue of finishSync. The
// plan is always emitted; if neither --dry-run nor --diff is set and
// the plan does not require explicit confirmation, an Execute runs
// and the Result is emitted as a second JSON document. Conflicts
// without --yes are treated like the human-path quit branch: the
// plan is emitted and no Execute is run, so callers can inspect the
// plan and re-invoke with --yes if they want to proceed.
func finishJSON(engine engineRunner, plan *sync.SyncPlan, out io.Writer, flags runFlags) error {
	if err := display.RenderPlanJSON(out, plan); err != nil {
		return err
	}

	if flags.DryRun || flags.Diff || plan.IsEmpty() {
		return nil
	}

	if shouldPromptConflicts(plan, flags.Yes) {
		return nil
	}

	result, err := engine.Execute(plan)
	if err != nil {
		return err
	}

	return display.RenderResultJSON(out, result)
}
