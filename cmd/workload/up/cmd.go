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

// Package up implements `dr workload up`: read the committed manifest, work
// out what differs from what is running, and apply only that. The command is
// the shell; the deploy lives in internal/workload/up.
package up

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/up"
	"github.com/spf13/cobra"
)

// Per-phase wait defaults. A deploy is slower than the shared defaults
// assume: an image build routinely runs for minutes.
const (
	defaultPollInterval = 5 * time.Second
	defaultPollTimeout  = 30 * time.Minute
)

// runFn is the deploy, swapped by this package's tests so the shell's own
// behaviour (flag refusal, stream separation, envelope shape) can be checked
// without a network.
var runFn = up.Run

// isStdinTerminalFn answers whether a person is there to be asked. A test
// harness always pipes stdin, so the tests that are about what happens on a
// terminal replace it.
var isStdinTerminalFn = reader.IsStdinTerminal

// upResult is the stable JSON shape emitted by --output-format json. BuildID
// is a pointer so it is null rather than empty when no build happened, which
// is the difference between "skipped" and "produced nothing".
type upResult struct {
	WorkloadID string      `json:"workloadId"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	Endpoint   string      `json:"endpoint"`
	ArtifactID string      `json:"artifactId"`
	BuildID    *string     `json:"buildId"`
	Action     string      `json:"action"`
	Locked     bool        `json:"locked"`
	Plan       up.PlanJSON `json:"plan"`
}

// buildID is the envelope's build reference: the id when a build ran, null
// when none did. An empty string would read as a build that produced nothing.
func buildID(id string) *string {
	if id == "" {
		return nil
	}

	return &id
}

type flags struct {
	dir    string
	yes    bool
	dryRun bool
	detach bool
	lock   bool
	force  bool

	// bindingFlags exist only to be refused. Cobra's own "unknown flag"
	// message would leave the user guessing where binding lives, and these
	// two are the obvious things to reach for.
	workloadID string
	name       string
}

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		f            flags
		poll         pollflags.Set
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Deploy this project, applying only what changed.",
		Long: `Read the committed .datarobot.yaml, compare it and the working tree against
what is running, and apply the difference.

The plan is printed, then carried out. Nothing to do is "Already up to date"
and exit 0. Use --dry-run to see the plan and stop, which is also the answer
to "what would this deploy" in CI.

Only fields the manifest mentions are managed. A setting the file never names
survives every deploy, and deleting a line stops managing that field rather
than reverting it. Change something in the UI that the file does name and the
next run puts it back, and says so.

With no manifest, a terminal opens the same setup wizard 'dr workload config'
runs and continues straight into the deploy. Without a terminal it is an
error: this command never deploys by guessing.

A manifest that names a published image deploys in one call. One that asks the
platform to build creates an artifact, pushes the working tree to it and waits
for an image first, so the first deploy of such a project takes as long as the
build does. A build the deploy believes would produce the image already on the
artifact is skipped; --force-build says otherwise.

Deploying onto a workload that already exists rolls it: a new version is made
from the file and swapped in, the endpoint does not change, and the version
already serving keeps serving until the new one is ready. When that version is
locked, meaning production, an interactive run asks for the workload name to
be typed back. A run with no terminal, or --yes, rolls without asking.

A change that moves only the sizing, such as a replica count or a resource
allocation, is applied in place instead. Nothing is built and no version is
made, because what the workload runs has not changed. A deploy that moves both
sends the sizing with the rollout, so the new version comes up with it.

Examples:
  dr workload up
  dr workload up --dry-run
  dr workload up --yes --output-format json`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return run(cmd, f, poll, outputFormat)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	addFlags(cmd, &f, &poll)

	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"yes":           f.yes || viperx.GetBool("yes"),
			"dry_run":       f.dryRun,
			"detach":        f.detach,
			"lock":          f.lock,
			"force_build":   f.force,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func addFlags(cmd *cobra.Command, f *flags, poll *pollflags.Set) {
	cmd.Flags().StringVar(&f.dir, "dir", "", "Project directory; the manifest is searched upward from here.")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false,
		"Do not prompt. With no manifest this is an error rather than a wizard, "+
			"and rolling a locked production version is not confirmed.")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print the plan and change nothing.")
	cmd.Flags().BoolVar(&f.detach, "detach", false, "Return once the deploy is requested; do not wait for it to serve.")
	cmd.Flags().BoolVar(&f.lock, "lock", false,
		"Lock the artifact that ends up live, making it permanent. Locking is one-way.")
	cmd.Flags().BoolVar(&f.force, "force-build", false,
		"Rebuild the image even when the working tree matches what was last synced.")

	cmd.Flags().StringVar(&f.workloadID, "workload-id", "", "")
	cmd.Flags().StringVar(&f.name, "name", "", "")
	_ = cmd.Flags().MarkHidden("workload-id")
	_ = cmd.Flags().MarkHidden("name")

	// Deliberately not pollflags.Register: it would add a --wait that
	// contradicts --detach.
	cmd.Flags().Var(pollflags.PositiveDuration(&poll.Interval, defaultPollInterval),
		"poll-interval", "How often to check on a deploy in progress.")
	cmd.Flags().Var(pollflags.PositiveDuration(&poll.Timeout, defaultPollTimeout),
		"poll-timeout", "How long to wait for a deploy before giving up.")
	_ = cmd.Flags().MarkHidden("poll-interval")
	_ = cmd.Flags().MarkHidden("poll-timeout")
}

func run(cmd *cobra.Command, f flags, poll pollflags.Set, format outputformat.OutputFormat) error {
	if err := checkFlags(cmd, f); err != nil {
		return err
	}

	dir, err := resolveDir(f.dir)
	if err != nil {
		return err
	}

	json := format == outputformat.OutputFormatJSON

	yes := f.yes || viperx.GetBool("yes")

	// JSON output implies non-interactive: a wizard drawn on stderr while
	// stdout is being parsed is a trap for whoever is parsing it.
	nonInteractive := yes || json || !isStdinTerminalFn()

	result, runErr := runFn(up.Options{
		Dir:            dir,
		NonInteractive: nonInteractive,
		DryRun:         f.dryRun,
		Detach:         f.detach,
		Lock:           f.lock,
		Confirm:        rollConfirm(cmd, yes),
		ForceBuild:     f.force,
		PollInterval:   poll.Interval,
		PollTimeout:    poll.Timeout,
		Stderr:         cmd.ErrOrStderr(),
		Spinner:        !json && !nonInteractive,
	})

	if runErr != nil && !reportable(result) {
		return runErr
	}

	if err := render(cmd, f, format, result); err != nil {
		return err
	}

	return runErr
}

// resolveDir defaults --dir to where the shell is standing.
func resolveDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine the current directory: %w", err)
	}

	return cwd, nil
}

// checkFlags refuses what the flags cannot mean.
//
// --workload-id and --name are the two people reach for out of habit. Binding
// a manifest to a workload is setup's job, and doing it here would mean two
// places that decide what a project deploys to.
//
// --lock with --detach is the one contradictory pair: an artifact is locked
// only once the workload it serves is running, and --detach returns before
// that, so accepting both would drop the lock in silence and hand back an
// artifact the caller believes is permanent.
func checkFlags(cmd *cobra.Command, f flags) error {
	for _, flag := range []string{"workload-id", "name"} {
		if cmd.Flags().Changed(flag) {
			return fmt.Errorf(
				"--%s is not a flag of this command: which workload a project deploys to is recorded in "+
					".datarobot.yaml. Use 'dr workload config --%s ...' to set it", flag, flag)
		}
	}

	if f.detach && f.lock {
		return errors.New(
			"--lock cannot be combined with --detach: an artifact is locked only after the workload it serves is running")
	}

	return nil
}

// rollConfirm is how a locked production roll gets its answer, and nil when
// there is nobody to ask or the answer is already in. --yes is that answer,
// and a run with no terminal has nobody to give one.
//
// Deliberately not keyed on --output json. That flag says how to format
// stdout; it is not consent, and folding it in here would mean
// `dr workload up --output json` on a terminal rolls production without a
// word. The question and its answer never touch stdout, so a run that asks
// still emits exactly one document.
func rollConfirm(cmd *cobra.Command, yes bool) func(question, want string) (bool, error) {
	if yes || !isStdinTerminalFn() {
		return nil
	}

	return typedConfirm(cmd)
}

// typedConfirm asks a question that only the exact expected word answers.
//
// It goes to stderr and reads a single line, like everything else the deploy
// says: stdout is the endpoint, or one JSON document, and a question printed
// into it would break whatever is parsing it. A y/n would not do here, since
// the one thing this is asked about is rolling production.
//
// The deploy calls it only on an interactive run, so it never has to decide
// whether prompting is allowed. An answer that cannot be read at all is a no,
// which leaves the workload running what it was running.
func typedConfirm(cmd *cobra.Command) func(question, want string) (bool, error) {
	return func(question, want string) (bool, error) {
		fmt.Fprint(cmd.ErrOrStderr(), question)

		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return false, fmt.Errorf("cannot read the answer: %w", err)
			}

			return false, nil
		}

		return strings.TrimSpace(scanner.Text()) == want, nil
	}
}

// reportable says whether a failed run left behind anything worth printing.
// A failure after the workload exists still reports it: losing the id because
// the wait timed out is how a deploy becomes unfindable. A build that failed
// before any workload existed is the same problem, since the build id is the
// only way back to the logs that say why. With no id at all an envelope of
// empty strings would be noise.
func reportable(result up.Result) bool {
	return result.WorkloadID != "" || result.BuildID != ""
}

func render(cmd *cobra.Command, f flags, format outputformat.OutputFormat, result up.Result) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "up", upResult{
			WorkloadID: result.WorkloadID,
			Name:       result.Name,
			Status:     result.Status,
			Endpoint:   result.Endpoint,
			ArtifactID: result.ArtifactID,
			BuildID:    buildID(result.BuildID),
			Action:     result.Action,
			Locked:     result.Locked,
			Plan:       result.Plan.JSON(),
		})
	}

	if f.dryRun {
		fmt.Fprintln(cmd.ErrOrStderr(), "\nDry run: nothing was changed.")

		return nil
	}

	// stdout carries the endpoint and nothing else, so it can be piped.
	if result.Endpoint != "" {
		fmt.Fprintln(cmd.OutOrStdout(), result.Endpoint)
	}

	return nil
}
