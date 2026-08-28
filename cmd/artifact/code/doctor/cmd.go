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
	"fmt"
	"path/filepath"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	wldoctor "github.com/datarobot/cli/internal/workload/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
)

func init() {
	// --yes is read directly from cobra; only the env var binds to viper so an
	// explicit --yes never leaks into drconfig.yaml.
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")
}

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

Exit code is 0 when no check FAILs (warnings are allowed) and 1 when at
least one check FAILs. Pass --output-format json for a machine-parseable
report on stdout.

Example:
  dr artifact code doctor
  dr artifact code doctor --dir ./service
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

	telemetry.TrackWith(c, func(cmd *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"yes":           cli.IsNonInteractive(cmd),
			"output_format": string(outputFormat),
		}
	})

	return c
}

// runDoctor executes one diagnosis: resolve the project directory, run the
// check suite, render the report, and exit 1 iff any check FAILed. The
// rendered report is the user-facing outcome, so a FAIL run returns
// cli.ErrSilent (with SilenceErrors set) instead of a second cobra error line.
func runDoctor(cmd *cobra.Command, outputFormat outputformat.OutputFormat) error {
	dirFlag, _ := cmd.Flags().GetString("dir")

	projectDir, err := resolveProjectDir(dirFlag)
	if err != nil {
		return err
	}

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

	// The six local checks in the pinned fixed order. Remote checks append
	// after these once their feature lands (fixed order contract).
	results := core.NewRunner(wldoctor.LocalChecks(projectDir)...).Run(cmd.Context())

	report := core.NewReport(projectDir, linkedArtifactID(projectDir), results)

	out := cmd.OutOrStdout()

	if outputFormat == outputformat.OutputFormatJSON {
		err = core.WriteJSON(out, report)
	} else {
		err = core.WriteText(out, report)
	}

	if err != nil {
		return err
	}

	if report.ExitCode() == 1 {
		// The report is already rendered on stdout; silence cobra's error
		// echo and exit 1 via the sentinel (main maps any RunE error to 1).
		// Per docs/development/telemetry.md, a command returning
		// cli.ErrSilent must carry SilenceErrors — set here so flag/usage
		// errors keep their explanatory message.
		cmd.SilenceErrors = true

		return cli.ErrSilent
	}

	return nil
}

// resolveProjectDir turns the --dir value (or its "." default) into the
// absolute project path used everywhere in the report. filepath.Abs is the
// pinned base behavior; a symlinked final component is resolved to its target
// so a project reached through a link reports the link's destination.
// Intermediate components stay as written, so an OS-level alias the user did
// not create (e.g. macOS /tmp → /private/tmp) never rewrites their path.
func resolveProjectDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}

	resolved, linkErr := filepath.EvalSymlinks(abs)
	if linkErr != nil || filepath.Base(resolved) == filepath.Base(abs) {
		// A missing or unreadable path keeps the Abs result — the checks
		// report the real condition. An unchanged final component means the
		// path itself is not a symlink.
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
