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

// Package doctor wires the sync-state diagnostics into the
// `dr artifact code doctor` Cobra command. It resolves flags, probes remote
// credentials non-fatally, runs the check suite from internal/workload/doctor
// through the generic framework Runner, renders the report as text or JSON,
// and maps any FAIL to exit code 1.
package doctor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	wldoctor "github.com/datarobot/cli/internal/workload/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

func init() {
	// --yes is read directly from cobra; only the env var binds to viper so an
	// explicit --yes never leaks into drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")
}

// Test seam: cmd_test.go reassigns this to stub the remote artifact fetch.
// Production wiring always leaves it pointing at workload.GetArtifact.
var getArtifactFn = workload.GetArtifact

// Cmd returns the cobra.Command for `dr artifact code doctor`. It inherits the
// artifact tree's DATAROBOT_CLI_FEATURE_WORKLOAD gate from its parent.
func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	c := &cobra.Command{
		Use:          "doctor",
		Short:        "Diagnose the artifact-code sync state of a project directory.",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Long: `Diagnose the '.datarobot/workload/' sync state of a project
directory without changing anything.

The doctor inspects the local sync state (linked artifact, config and
manifest health, config/manifest agreement, interrupted rollbacks, and the
sync lock) and reports each check as OK, WARN, FAIL, or SKIP with a concrete
remedy for anything that needs attention. It is a read-only diagnostic: no
prompt is issued, no file is written, and no remote call is made unless
remote checks apply.

Pass --fix to attempt the safe local repairs (rebuild the manifest from
config, restore an interrupted rollback, clear a stale sync lock), then
re-run every check and report the post-fix state. Nothing is ever written to
the server, and a live sync holding the lock gates all repairs. --fix and
--relink are mutually exclusive.

Pass --relink <new-artifact-id> to repoint the project at a different
artifact with a fresh sync baseline. The target must exist, be a draft
(not locked), and be a service-type artifact. An interactive confirm prompt
defaults to No (use --yes to skip it); the working tree is never touched and
no server writes are made. The relink is logged to history.log.

Exit code is 0 when no check FAILs (warnings are allowed) and 1 when at
least one check FAILs. Pass --output-format json for a machine-parseable
report on stdout.

Example:
  dr artifact code doctor
  dr artifact code doctor --dir ./service
  dr artifact code doctor --fix
  dr artifact code doctor --relink <new-artifact-id>
  dr artifact code doctor --output-format json`,
		// No PreRunE on purpose: a read-only diagnostic must never abort on
		// auth or launch the interactive login wizard. Auth is probed softly
		// inside RunE instead (see softAuthProbe).
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return runDoctor(cmd, outputFormat)
		},
	}

	outputformat.AddFlag(c, &outputFormat)

	c.Flags().String("dir", ".", "Project directory to diagnose (default: current directory).")

	// Read-only diagnosis never prompts, so --yes changes nothing today; it
	// exists so scripts can pass it uniformly and so repair modes added later
	// share the same non-interactive switch.
	c.Flags().BoolP(cli.YesFlagName, "y", false, "Never prompt (read-only diagnosis never prompts anyway).")

	c.Flags().Bool("fix", false,
		"Attempt safe local repairs (rebuild the manifest from config, restore an "+
			"interrupted rollback, clear a stale sync lock), then re-run the checks.")

	c.Flags().String("relink", "",
		"Repoint the project at <new-artifact-id> with a fresh sync baseline. "+
			"The target must exist, be a draft, and be a service-type artifact. "+
			"Mutually exclusive with --fix.")

	c.MarkFlagsMutuallyExclusive("fix", "relink")

	telemetry.TrackWith(c, func(cmd *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"yes":           cli.IsNonInteractive(cmd),
			"output_format": string(outputFormat),
			"fix":           fixFlagChanged(cmd),
			"relink":        relinkFlagChanged(cmd),
		}
	})

	return c
}

// runDoctor executes one diagnosis: resolve the project directory, run the
// check suite, render the report, and exit 1 iff any check FAILed. With
// --fix, the safe local repairs run first and the reported checks (and exit
// code) reflect the POST-fix state. With --relink, the project is repointed
// at a new artifact (fresh BASE reset) before the checks re-run. The rendered
// report is the user-facing outcome, so a FAIL run returns cli.ErrSilent
// (with SilenceErrors set) instead of a second cobra error line.
func runDoctor(cmd *cobra.Command, outputFormat outputformat.OutputFormat) error {
	fix, _ := cmd.Flags().GetBool("fix")

	relinkID, _ := cmd.Flags().GetString("relink")

	// --fix and --relink are mutually exclusive: cobra's
	// MarkFlagsMutuallyExclusive handles the usage error, but the explicit
	// check stays as a belt-and-suspenders guard (and gives a clearer
	// message than cobra's generic one).
	if fix && cmd.Flags().Changed("relink") {
		return errors.New("--fix and --relink are mutually exclusive; use one or the other")
	}

	dirFlag, _ := cmd.Flags().GetString("dir")

	projectDir, err := resolveProjectDir(dirFlag)
	if err != nil {
		return err
	}

	actions, relinkErr := runRepairPhase(cmd, projectDir, fix, relinkID)

	// Soft auth probe: resolve remote credentials without prompting and
	// without writing any config file. Local checks never need auth; the
	// remote checks (wired by their own feature) will SKIP with a
	// connectivity remedy when no usable credentials are found.
	creds, authed := softAuthProbe()

	if authed {
		log.Debug("doctor resolved remote credentials", "endpoint", creds.Endpoint)
	} else {
		log.Debug("doctor found no remote credentials; remote checks will report SKIP")
	}

	// The complete check suite in the pinned fixed order: six local checks
	// then the four remote checks. The remote checks share one artifact
	// fetch through the getArtifactFn seam. After --fix/--relink this is the
	// post-repair state, so both the report and the exit code describe what
	// remains.
	results := core.NewRunner(
		wldoctor.Checks(projectDir, wldoctor.ArtifactGetterFunc(getArtifactFn))...,
	).Run(cmd.Context())

	report := core.NewReport(projectDir, linkedArtifactID(projectDir), results)

	report.Actions = actions

	if err := renderReport(cmd, outputFormat, report); err != nil {
		return err
	}

	// Print relink abort errors to stderr (for not-linked and API-unreachable
	// cases the user needs a message; for other aborts the actions array
	// already describes the reason). In JSON mode this keeps stdout pure.
	if relinkErr != nil && !errors.Is(relinkErr, wldoctor.ErrRelinkAbort) {
		fmt.Fprintln(cmd.ErrOrStderr(), relinkErr)
	}

	if relinkErr != nil || report.ExitCode() == 1 {
		cmd.SilenceErrors = true

		return cli.ErrSilent
	}

	return nil
}

// runRepairPhase executes the --fix or --relink repair phase and returns the
// actions (nil for read-only runs) and any relink error (nil for --fix and
// read-only runs).
func runRepairPhase(cmd *cobra.Command, projectDir string, fix bool, relinkID string) (*[]core.Action, error) {
	if fix {
		performed := wldoctor.RunFix(cmd.Context(), projectDir)

		return &performed, nil
	}

	if relinkID != "" {
		performed, rErr := runRelinkPhase(cmd, projectDir, relinkID)

		if performed != nil {
			return &performed, rErr
		}

		return nil, rErr
	}

	return nil, nil
}

// renderReport writes the report to stdout as text or JSON.
func renderReport(cmd *cobra.Command, outputFormat outputformat.OutputFormat, report core.Report) error {
	out := cmd.OutOrStdout()

	if outputFormat == outputformat.OutputFormatJSON {
		return core.WriteJSON(out, report)
	}

	return core.WriteText(out, report)
}

// resolveProjectDir turns the --dir value (or its "." default) into the
// absolute project path used everywhere in the report. filepath.Abs is the
// pinned base behavior; a symlinked final component is resolved to its target
// so a project reached through a link reports the link's destination.
// Intermediate components stay as written, so an OS-level alias the user did
// not create (e.g. macOS /tmp → /private/tmp) never rewrites their path.
//
// Symlink detection uses os.Lstat on the final component rather than a
// basename comparison of the EvalSymlinks result: a symlink whose target
// directory happens to share the link's basename would defeat a basename
// check but is correctly detected by Lstat.
func resolveProjectDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}

	// Lstat the final component without following it: only a symlink on the
	// last element triggers resolution. Intermediate symlinks (e.g. macOS
	// /tmp → /private/tmp) are transparently resolved by the OS during Lstat
	// but do not cause the final component to be reported as a symlink, so
	// the user's path stays as written. A missing or unreadable path keeps
	// the Abs result — the checks report the real condition.
	info, statErr := os.Lstat(abs)
	if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		return abs, nil
	}

	resolved, linkErr := filepath.EvalSymlinks(abs)
	if linkErr != nil {
		return abs, nil
	}

	return resolved, nil
}

// linkedArtifactID reads the linked artifact id from the project's state
// config for the report header. Any read failure or an empty id (empty ≈ nil
// normalization) reports the project as unlinked.
func linkedArtifactID(projectDir string) *string {
	cfg, err := wapi.LoadConfig(projectDir)
	if err != nil || cfg.ArtifactID == "" {
		return nil
	}

	id := cfg.ArtifactID

	return &id
}

// remoteCreds holds non-fatally resolved remote credentials for the doctor's
// remote checks.
type remoteCreds struct {
	Endpoint string
	Token    string
}

// softAuthProbe resolves remote credentials without prompting the user and
// without writing any configuration file. The doctor is a read-only
// diagnostic, so it must never trigger the interactive login wizard that
// auth.EnsureAuthenticated would and must never touch drconfig.yaml.
//
// Resolution mirrors the CLI's auth precedence: a complete
// DATAROBOT_ENDPOINT/DATAROBOT_API_TOKEN environment pair wins; otherwise the
// stored drconfig.yaml profile is used. A partial env pair is ignored (an
// incomplete pair is an explicit but unusable request). The probe performs no
// network I/O: reachability is judged by the remote checks themselves, which
// report SKIP with a `dr auth login` remedy when these credentials do not
// work.
func softAuthProbe() (remoteCreds, bool) {
	env := auth.GetEnvCredentials()

	if env.Endpoint != "" && env.Token != "" {
		return remoteCreds{Endpoint: env.Endpoint, Token: env.Token}, true
	}

	endpoint := config.GetBaseURL()
	token := viperx.GetString(config.DataRobotAPIKey)

	if endpoint == "" || token == "" {
		return remoteCreds{}, false
	}

	return remoteCreds{Endpoint: endpoint, Token: token}, true
}

// runRelinkPhase executes the relink operation and returns the actions and
// error. Extracted from runDoctor to keep cyclomatic complexity manageable.
func runRelinkPhase(cmd *cobra.Command, projectDir, relinkID string) ([]core.Action, error) {
	return wldoctor.RunRelink(cmd.Context(), wldoctor.RelinkOptions{
		ProjectDir:    projectDir,
		NewArtifactID: relinkID,
		Store:         wldoctor.ArtifactGetterFunc(getArtifactFn),
		Confirm:       makeRelinkConfirm(cmd),
	})
}

// makeRelinkConfirm builds the confirm function for the relink operation.
//
// Interactive (TTY, no --yes): the warning and a [y/N] prompt are written to
// stderr; only an explicit "y" or "yes" proceeds (empty Enter declines — this
// is the bespoke default-No prompt, NOT reader.AskYesNo which treats empty
// Enter as Yes). Ctrl-C/EOF at the prompt also declines.
//
// Non-interactive (--yes or non-TTY): the warning is printed to stderr and the
// relink proceeds. In JSON mode this keeps stdout pure (all human text to
// stderr).
func makeRelinkConfirm(cmd *cobra.Command) wldoctor.RelinkConfirmFunc {
	nonInteractive := cli.IsNonInteractive(cmd)

	stderr := cmd.ErrOrStderr()

	return func(warning string) bool {
		if nonInteractive || !reader.IsStdinTerminal() {
			fmt.Fprintln(stderr, warning)

			return true
		}

		fmt.Fprintln(stderr, warning)

		fmt.Fprint(stderr, "Proceed? [y/N] ")

		line, err := reader.ReadString()
		if err != nil {
			// Ctrl-C, EOF, or cancelreader error: treat as decline.
			return false
		}

		answer := strings.TrimSpace(strings.ToLower(line))

		return answer == "y" || answer == "yes"
	}
}

// fixFlagChanged reports whether --fix was explicitly set, for telemetry.
func fixFlagChanged(cmd *cobra.Command) bool {
	changed, _ := cmd.Flags().GetBool("fix")

	return changed
}

// relinkFlagChanged reports whether --relink was explicitly set, for telemetry.
func relinkFlagChanged(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("relink")
}
