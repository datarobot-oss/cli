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

// Package importcmd implements the `dr workload env import` verb. The
// directory is named `import` (matching the CLI surface) but the package is
// named importcmd because `import` is a Go keyword, not just a predeclared
// identifier -- unlike cmd/workload/del's shadowing concern, this one is a
// hard syntax error, not just a footgun.
package importcmd

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/cmd/workload/env/internal/envparse"
	"github.com/datarobot/cli/cmd/workload/env/internal/rollout"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// defaultEnvFile is read when --file is not given, matching the universal
// dotenv convention of a file named exactly this in the current directory.
const defaultEnvFile = ".env"

// Mirrors cmd/workload/env/set's poll defaults -- replacement settling is
// closer to a rolling redeploy than a container build.
const (
	defaultPollInterval = 20 * time.Second
	defaultPollTimeout  = 20 * time.Minute
)

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	var poll pollflags.Set

	cmd := &cobra.Command{
		Use:   "import <workload-id> [--file <path>]",
		Short: "Import environment variables from a .env file onto a workload.",
		Long: `Import environment variables from a .env file onto the artifact a
workload is running, then roll the workload onto the result.

Only the workload's primary container is affected. Artifacts with
additional (sidecar) containers are not yet supported by this command.

Reads .env in the current directory by default; pass --file to import from
a different path. The file is parsed with standard dotenv syntax (blank
lines and '#'-prefixed comments are ignored; values may be quoted). Every
variable found in the file is applied together in one call, exactly as if
they had all been passed to 'dr workload env set' at once -- including the
NAME=dr-credential:<credential-id>/<credential-key> syntax for referencing a
stored credential instead of a literal value (see 'dr workload env set
--help' for the full syntax and rationale). If a name from the file already
has a value set on the workload, the file's value wins -- this is an
ordinary upsert by name, never a conflict error.

The same validation as 'env set' applies to every variable found in the
file, checked before anything is written: NAME must be a valid environment
variable name (letters, digits, '_', '-', or '.'; cannot start with a
digit), and any dr-credential:<id>/<key> reference must name a credential
that actually exists.

Concurrent edits to the SAME workload can silently clobber each other: this
command reads the current spec, merges your change, and writes the whole
spec back, with no conflict detection. Avoid running 'env set'/'env
import'/'env delete' against the same workload from two sessions at once --
whichever write lands last wins, and the other's change is lost without any
error.

If the workload's current artifact is a draft, the change is applied to it
in place. If it is locked (locking is one-way and irreversible), a new
artifact is cloned from it and edited instead -- the workload itself is not
touched until the rollout below runs.

Unless --stage is given, this first checks that the workload doesn't
already have a replacement in progress and refuses to proceed if it does
(retry once it settles) -- before reading the file or touching any
artifact, so a locked workload with a rollout already underway doesn't get
a wasted throwaway clone created for an edit that couldn't deploy anyway.

Without --stage, this then asks for confirmation (skip with --yes) and
triggers a rolling replacement of the workload onto the resulting artifact.
With --stage, the artifact is prepared but not deployed: no confirmation is
needed, nothing about the running workload changes, and the in-progress-
replacement check above is skipped -- staging never touches the live
rollout machinery, so it's safe even while another replacement settles.

Example:
  dr workload env import 68b0c1d2e3f4a5b6c7d8e9f0
  dr workload env import 68b0c1d2e3f4a5b6c7d8e9f0 --file production.env --wait
  dr workload env import 68b0c1d2e3f4a5b6c7d8e9f0 --stage`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return run(cmd, outputFormat, args, poll)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	pollflags.RegisterWithDefaults(cmd, &poll, defaultPollInterval, defaultPollTimeout, "Poll until the replacement settles.")

	cmd.Flags().String("file", "", "Path to the env file to import (default: .env in the current directory).")
	cmd.Flags().Bool("stage", false, "Apply the edit without triggering a rollout.")
	cmd.Flags().BoolP("yes", "y", false, "Skip the rollout confirmation prompt.")

	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, args []string) map[string]any {
		stageFlag, _ := cmd.Flags().GetBool("stage")
		yesFlag, _ := cmd.Flags().GetBool("yes")
		fileFlag, _ := cmd.Flags().GetString("file")

		return map[string]any{
			"workload_id":      telemetry.FirstArg(args),
			"used_custom_file": fileFlag != "",
			"stage":            stageFlag,
			"yes":              yesFlag || viperx.GetBool("yes"),
			"wait":             poll.Wait,
			"output_format":    string(outputFormat),
		}
	})

	return cmd
}

func run(cmd *cobra.Command, format outputformat.OutputFormat, args []string, poll pollflags.Set) error {
	workloadID := args[0]

	stageFlag, _ := cmd.Flags().GetBool("stage")

	// Fail fast, before reading the file or touching any artifact, if a
	// rollout couldn't happen right now anyway. Skipped when staging:
	// staging never touches the live rollout machinery, so it's safe to
	// prepare a follow-up edit while an earlier replacement settles.
	if !stageFlag {
		if err := rollout.GuardNoActiveReplacement(workloadID); err != nil {
			return err
		}
	}

	fileFlag, _ := cmd.Flags().GetString("file")

	filePath := fileFlag
	if filePath == "" {
		filePath = defaultEnvFile
	}

	vars, err := loadVarsFromFile(filePath)
	if err != nil {
		return err
	}

	if err := envparse.ValidateCredentialReferences(vars); err != nil {
		return err
	}

	wl, artifact, err := workload.ResolveWorkloadArtifact(workloadID)
	if err != nil {
		return err
	}

	targetArtifactID, needsLock, err := workload.UpsertEnvironmentVars(artifact.ID, vars)
	if err != nil {
		return err
	}

	yesFlag, _ := cmd.Flags().GetBool("yes")

	return rollout.Apply(cmd, format, wl.ID, targetArtifactID, needsLock, rollout.Options{
		Stage: stageFlag,
		Yes:   yesFlag || viperx.GetBool("yes"),
		Poll:  poll,
	})
}

// loadVarsFromFile reads and parses path as a dotenv file (via godotenv,
// already a dependency of this CLI through internal/envbuilder) and returns
// a name-sorted list of EnvironmentVar. Sorting is purely for deterministic,
// reproducible output: Go's map iteration order is randomized, and the
// resulting PATCH body would otherwise vary from run to run for no reason.
// Applies the identical NAME/credential-reference validation as
// `env set`'s positional arguments, via envparse.BuildVar.
func loadVarsFromFile(path string) ([]workload.EnvironmentVar, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	parsed, err := godotenv.Unmarshal(string(contents))
	if err != nil {
		return nil, fmt.Errorf("parse env file %s: %w", path, err)
	}

	if len(parsed) == 0 {
		return nil, fmt.Errorf("no environment variables found in %s", path)
	}

	names := make([]string, 0, len(parsed))
	for name := range parsed {
		names = append(names, name)
	}

	sort.Strings(names)

	vars := make([]workload.EnvironmentVar, 0, len(names))

	for _, name := range names {
		ev, err := envparse.BuildVar(name, parsed[name])
		if err != nil {
			return nil, err
		}

		vars = append(vars, ev)
	}

	return vars, nil
}
