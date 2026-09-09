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
	"strings"

	"github.com/datarobot/cli/cmd/artifact/code/internal/dirprompt"
	"github.com/datarobot/cli/cmd/artifact/code/internal/format"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
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
	StateMigrationNotice() string
	IgnoreFileNotice() string
	LockedNotice() string
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
	// AskDir prompts for the project directory when neither --dir nor a
	// non-interactive mode supplies one. A seam so tests can assert the
	// preview modes never reach it.
	AskDir func(label, defaultVal string) (string, error)
}

// runFlags is the parsed view of the boolean flags that gate
// finishSync's render/prompt/execute decisions. Grouped so the inner
// helpers don't carry a three-bool tail through every signature.
type runFlags struct {
	DryRun bool
	Diff   bool
	Yes    bool
	// AcceptRemote opts a non-interactive run into letting the remote
	// overwrite or delete local files. Without it, such a run is refused
	// rather than silently destroying local work (RAPTOR-19348).
	AcceptRemote bool
}

// Preview reports whether this run only shows a plan and writes nothing, on
// either the local or the remote side. Such a run must never block on a
// prompt: an operation that by definition changes nothing has no confirmation
// to ask for (RAPTOR-19348).
func (f runFlags) Preview() bool {
	return f.DryRun || f.Diff
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
		AskDir:   dirprompt.AskWithDefault,
	}
}

func init() {
	// --yes is read directly from cobra; only the env var binds to viper
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")
}

// Cmd returns the cobra.Command for `dr artifact code sync`.
func Cmd() *cobra.Command {
	return cmdWithDeps(defaultDeps())
}

func cmdWithDeps(deps Deps) *cobra.Command {
	var outputFormat outputformat.OutputFormat

	c := &cobra.Command{
		Use:          "sync",
		Short:        "Push and pull code changes between this directory and the linked artifact.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `Synchronize the linked DataRobot artifact with the project
directory. Computes a three-way diff against the last known state and
applies the resulting plan in a single versioned step.

Before the remote replaces or deletes any local file, your version is
saved as a *.LOCAL.<timestamp> copy, so nothing local is ever lost
without a recoverable backup. When a file changed both locally and on
the remote (a conflict), the run asks first; run non-interactively
(--yes or in CI) a conflict is refused unless you pass --accept-remote,
so an automated sync never silently overwrites your local changes. A
plain pull of a remote change you had not touched applies without
prompting (still backed up to *.LOCAL).

Use --dry-run to preview the plan without writing anything; --diff to
also print per-file unified diffs. Both modes exit before any remote
write and never prompt, so they are safe to run unattended. --yes
auto-confirms the post-plan prompt and skips any interactive directory
prompt.

Run 'dr artifact code init <artifact-id>' first to link a project
directory to an artifact.

Example:
  dr artifact code sync
  dr artifact code sync --dry-run
  dr artifact code sync --diff
  dr artifact code sync --yes
  dr artifact code sync --yes --accept-remote
  dr artifact code sync --output-format json`,
		PreRunE: auth.EnsureAuthenticatedE,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runSync(cmd, outputFormat, deps)
		},
	}

	outputformat.AddFlag(c, &outputFormat)

	c.Flags().String("dir", "", "Project directory (default: current directory).")
	c.Flags().Bool("dry-run", false, "Show plan, no writes.")
	c.Flags().Bool("diff", false, "Show plan + per-file unified diffs, no writes.")
	c.Flags().BoolP(cli.YesFlagName, "y", false, "Skip interactive prompts; auto-confirm.")
	c.Flags().Bool("accept-remote", false,
		"Allow the remote to overwrite or delete local files in a non-interactive run "+
			"(your versions are still saved as *.LOCAL copies).")
	c.MarkFlagsMutuallyExclusive("dry-run", "diff")

	telemetry.TrackWith(c, func(cmd *cobra.Command, _ []string) map[string]any {
		flags := parseRunFlags(cmd)

		return map[string]any{
			"dry_run":       flags.DryRun,
			"diff":          flags.Diff,
			"yes":           flags.Yes,
			"accept_remote": flags.AcceptRemote,
			"output_format": string(outputFormat),
		}
	})

	return c
}

func runSync(cmd *cobra.Command, outputFormat outputformat.OutputFormat, deps Deps) error {
	flags := parseRunFlags(cmd)

	dirFlag, _ := cmd.Flags().GetString("dir")

	ask := deps.AskDir
	if ask == nil {
		ask = dirprompt.AskWithDefault
	}

	// A preview writes nothing, so it resolves the directory without asking:
	// --dry-run used to block on this prompt before ever reaching the plan.
	dir, err := dirprompt.ResolveDir(dirFlag, flags.Yes || flags.Preview(), ask)
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

	// Stderr for all of them: they are never part of a command's data output,
	// and stdout has to stay parseable under --output-format json.
	format.StateNotice(cmd.ErrOrStderr(), engine.StateMigrationNotice())
	format.StateNotice(cmd.ErrOrStderr(), engine.IgnoreFileNotice())
	format.StateNotice(cmd.ErrOrStderr(), engine.LockedNotice())

	if engine.StaleRollbackRestored() {
		fmt.Fprintln(cmd.ErrOrStderr(), tui.DimStyle.Render("Recovered from interrupted sync. Working tree restored."))
	}

	return finishSync(cmd, engine, plan, outputFormat, flags, deps)
}

// parseRunFlags reads the cobra flags once and folds the
// DATAROBOT_CLI_NON_INTERACTIVE env-var override into Yes, so the
// downstream helpers see a single source of truth.
func parseRunFlags(cmd *cobra.Command) runFlags {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	diff, _ := cmd.Flags().GetBool("diff")

	acceptRemote, _ := cmd.Flags().GetBool("accept-remote")

	return runFlags{
		DryRun:       dryRun,
		Diff:         diff,
		Yes:          cli.IsNonInteractive(cmd),
		AcceptRemote: acceptRemote,
	}
}

// finishSync handles the render → optional prompt → execute → render
// tail of the command. Pulled out so runSync's early-return paths
// (auth, lock, plan errors) stay flat.
func finishSync(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan, outputFormat outputformat.OutputFormat, flags runFlags, deps Deps) error {
	out := cmd.OutOrStdout()

	if outputFormat == outputformat.OutputFormatJSON {
		return finishJSON(engine, plan, out, flags)
	}

	if err := renderHumanPlan(cmd, engine, plan, flags.Diff); err != nil {
		return err
	}

	if flags.DryRun || flags.Diff || plan.IsEmpty() {
		return nil
	}

	proceed, err := gateLocalOverwrite(cmd, engine, plan, flags, deps)
	if err != nil {
		return err
	}

	if !proceed {
		return nil
	}

	result, err := engine.Execute(plan)
	if err != nil {
		return err
	}

	return display.PrintResult(out, result)
}

// gateLocalOverwrite decides whether a run that would overwrite or delete local
// files may proceed. A plan that only uploads passes straight through. When it
// would write the working tree from the remote side, an interactive run asks
// first (default No), and a non-interactive run is refused unless --accept-remote
// opted in — so CI fails loud instead of silently rewriting its checkout
// (RAPTOR-19348). Either way, the files it would replace are backed up to
// *.LOCAL before the remote lands.
func gateLocalOverwrite(cmd *cobra.Command, engine engineRunner, plan *sync.SyncPlan, flags runFlags, deps Deps) (bool, error) {
	// Only conflicts gate: a REMOTE_MODIFIED or REMOTE_DELETED file is
	// byte-identical to the last sync (local == base), so pulling it is a
	// fast-forward with no unsaved work to lose. Those still get a .LOCAL
	// backup in the engine, but they do not prompt or refuse — otherwise a
	// routine "pull a teammate's change" would block, and CI would reflexively
	// paste --accept-remote and defeat the gate for the case that matters. A
	// conflict is the case that matters: both sides changed the same file.
	if !plan.HasConflicts() {
		return true, nil
	}

	if flags.Yes {
		if flags.AcceptRemote {
			return true, nil
		}

		return false, conflictRefusedError(plan)
	}

	choice, err := promptOverwriteMenu(cmd, engine, plan, deps.ReadLine)
	if err != nil {
		return false, err
	}

	return choice == promptSync, nil
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

// overwriteRefusedError is the non-interactive refusal: it names the local
// files the remote would replace or delete and the two ways forward, so a CI
// log says exactly what went wrong and how to proceed on purpose.
func conflictRefusedError(plan *sync.SyncPlan) error {
	paths := plan.ConflictPaths()

	return fmt.Errorf(
		"refusing to overwrite local changes: %d file(s) changed both locally and on the remote, "+
			"and no confirmation is possible non-interactively:\n%s\n"+
			"Re-run with --accept-remote to let the remote win (your versions are saved as *.LOCAL copies), "+
			"or --dry-run to inspect the plan first",
		len(paths), formatPathList(paths))
}

// formatPathList renders up to listCap indented paths, summarizing the rest so
// a plan touching hundreds of files does not bury the message.
func formatPathList(paths []string) string {
	const listCap = 10

	shown := paths
	more := 0

	if len(shown) > listCap {
		more = len(shown) - listCap
		shown = shown[:listCap]
	}

	var b strings.Builder

	for _, p := range shown {
		fmt.Fprintf(&b, "  %s\n", p)
	}

	if more > 0 {
		fmt.Fprintf(&b, "  ... and %d more\n", more)
	}

	return strings.TrimRight(b.String(), "\n")
}

// finishJSON is the --output-format=json analogue of finishSync. It emits
// exactly one JSON document: the plan at the top level and, when a
// version-writing Execute runs, its result under a "result" key. Emitting the
// plan and the result as two concatenated documents broke every consumer's
// json.loads (RAPTOR-19348).
//
// An Execute runs only when neither preview mode is set, the plan is non-empty,
// and it does not require explicit confirmation. Conflicts without --yes are
// treated like the human-path quit branch: the plan is emitted and no Execute
// is run, so callers can inspect it and re-invoke with --yes to proceed.
func finishJSON(engine engineRunner, plan *sync.SyncPlan, out io.Writer, flags runFlags) error {
	locked := engine.LockedNotice() != ""

	if flags.Preview() || plan.IsEmpty() {
		return display.RenderSyncJSON(out, plan, nil, locked)
	}

	if plan.HasConflicts() {
		// Without --yes there is no confirmation channel in JSON mode, so the
		// plan is emitted and nothing runs. The document carries "refused": true
		// so a consumer sees the run applied nothing — including its uploads —
		// rather than inferring it from an absent "result"; a script re-invokes
		// with --yes --accept-remote to proceed. With --yes but no opt-in, the
		// run is refused loudly (non-zero exit) rather than silently letting the
		// remote win over local changes (RAPTOR-19348).
		if !flags.Yes {
			return display.RenderRefusedJSON(out, plan, locked)
		}

		if !flags.AcceptRemote {
			return conflictRefusedError(plan)
		}
	}

	// The document is written only after Execute succeeds, so a failed sync
	// leaves stdout empty and reports the error on stderr with a non-zero exit.
	// This differs from the old two-document output, which emitted the plan
	// before Execute and so left a plan on stdout even when the sync failed:
	// with a single document, stdout carries a result only when there is one,
	// and a consumer keys off the exit status rather than parsing a document
	// that describes what was attempted rather than what happened.
	result, err := engine.Execute(plan)
	if err != nil {
		return err
	}

	return display.RenderSyncJSON(out, plan, result, locked)
}
