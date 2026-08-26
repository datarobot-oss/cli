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

// Package config implements `dr workload config`: the setup wizard that
// writes the committed .datarobot.yaml `dr workload up` deploys from. The
// command is the shell; the flow itself lives in internal/workload/wizard so
// `up` can run the identical wizard when it finds no manifest.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/spf13/cobra"
)

// configResult is the stable JSON shape emitted by --output-format json.
type configResult struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	WorkloadID string `json:"workloadId"`
	// CreateOnUp is true when no existing workload was bound, so the workload
	// is created by the first `dr workload up`.
	CreateOnUp bool   `json:"createOnUp"`
	BuildMode  string `json:"buildMode"`
	// Action is created, unchanged or planned. A caller keying on it has to be
	// able to tell a dry run's planned from a file that now exists.
	Action string `json:"action"`
	// EnvKeysListed is how many .env variables the file carries and
	// EnvSecretsPending how many of those still name the placeholder instead
	// of a credential id. Without them a pipeline reading only this envelope
	// would see a created manifest and no sign that it cannot deploy yet.
	EnvKeysListed     int `json:"envKeysListed"`
	EnvSecretsPending int `json:"envSecretsPending"`
	// EnvLiterals names the variables whose values the file carries in the
	// clear, so an audit step can check them without parsing the YAML.
	EnvLiterals []string `json:"envLiterals"`
}

// flags is the flag set, held together so the run path reads as one answer
// set rather than eighteen lookups.
type flags struct {
	dir        string
	dockerfile string
	dryRun     bool
	yes        bool
	answers    wizard.Answers
}

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		f            flags
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Set up this project for `dr workload up`.",
		Long: `Answer a handful of questions and write the committed .datarobot.yaml
that 'dr workload up' deploys from.

The file is the platform's own workload-create spec, plus the workloadId
binding the CLI manages for you. Setup is a one-time act: with a manifest
already present, this command prints its path and exits, because editing
twelve lines of YAML beats re-answering eight questions. Delete the file to
start over.

Defaults come from wherever the truth already lives: a Dockerfile and its
EXPOSE line for a fresh project, the workload's own live spec when you bind
to one. Binding downloads that spec into the file, so a deploy from it
cannot quietly downgrade something already running.

Without a terminal, or with --yes or --output-format json, nothing prompts:
the flags answer every question and a missing one is an error that names it.
The Dockerfile track needs no flags at all when ./Dockerfile exists.

Examples:
  dr workload config
  dr workload config --yes --name my-app
  dr workload config --yes --build-mode image --image registry/team/app:v1
  dr workload config --yes --build-mode generated \
    --execution-environment "[DataRobot] Python 3.12 Applications Base" \
    --entrypoint "uvicorn app:app --host 0.0.0.0 --port 8080"`,
		Args:         cobra.NoArgs,
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			return run(cmd, f, outputFormat)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	addFlags(cmd, &f)

	// Only the env var binds to viper; --yes is read from cobra so it never
	// persists into drconfig.yaml.
	_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
		return map[string]any{
			"yes":           f.yes || viperx.GetBool("yes"),
			"type":          f.answers.Type,
			"build_mode":    f.answers.BuildMode,
			"bound":         f.answers.WorkloadID != "",
			"dry_run":       f.dryRun,
			"output_format": string(outputFormat),
		}
	})

	return cmd
}

func addFlags(cmd *cobra.Command, f *flags) {
	cmd.Flags().StringVar(&f.dir, "dir", "", "Project directory (default: current directory).")
	// Not the --yes of `dr dotenv setup`, which invents an answer for
	// everything it was not given. Here the flags are the answers, and the
	// only guessing is what the project directory can be read for, so the
	// help text has to say that rather than leave "defaults" to be read as
	// the other command's meaning.
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false,
		"Do not prompt; answer every question from flags and what the project directory shows. "+
			"A question neither of those answers is an error naming the flag that would settle it.")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print the manifest and write nothing.")
	cmd.Flags().BoolVar(&f.answers.SkipEnv, "skip-env", false,
		"Do not carry the project's .env into the manifest. By default its variables are written there: "+
			"ordinary values as literals, secrets as credential references you complete later.")

	cmd.Flags().StringVar(&f.answers.WorkloadID, "workload-id", "", "Bind an existing workload by id. Exclusive with --name.")
	cmd.Flags().StringVar(&f.answers.Name, "name", "", "Name a new workload, created by the first `dr workload up`.")
	cmd.Flags().StringVar(&f.answers.Type, "type", "", "Workload kind: service or agent (default service).")
	cmd.Flags().BoolVar(&f.answers.A2AEnabled, "a2a-enabled", false,
		"Agent only: publish the app's A2A agent card to the tenant-wide agent registry.")

	cmd.Flags().StringVar(&f.answers.BuildMode, "build-mode", "", "Image source: dockerfile, generated or image.")
	cmd.Flags().StringVar(&f.dockerfile, "dockerfile", wizard.DefaultDockerfilePath,
		"Dockerfile to build. Only the project root's ./Dockerfile is supported.")
	cmd.Flags().StringVar(&f.answers.ExecutionEnvironment, "execution-environment", "",
		"Base image, by name or id. Required with --build-mode generated.")
	cmd.Flags().StringVar(&f.answers.Entrypoint, "entrypoint", "",
		"Command that starts the app, parsed shell-style. Required with --build-mode generated.")
	cmd.Flags().StringVar(&f.answers.Image, "image", "", "Published image URI. Required with --build-mode image.")

	cmd.Flags().IntVar(&f.answers.Port, "port", 0, "Container port (default: the Dockerfile's EXPOSE, else 8080).")
	cmd.Flags().StringVar(&f.answers.HealthPath, "health", "",
		"Readiness path, e.g. /health. Without it no probe is written: a probe has to be written for the app "+
			"it probes, and a wrong one stops a healthy deploy from ever going ready.")
	// A flag of its own rather than an empty --health. Declining a probe is a
	// different act from not mentioning one, and a flag that says so is both
	// greppable in a CI script and impossible to type by accident.
	// Deliberately about the file rather than about the running workload: `up`
	// leaves out what the file leaves out, so a probe already deployed goes
	// only when the artifact is rebuilt for some other reason.
	cmd.Flags().BoolVar(&f.answers.NoProbe, "no-readiness-probe", false,
		"Write no readiness probe, so traffic is not gated on a health check. Exclusive with --health. "+
			"With --workload-id it drops the probe from the manifest; a deployed probe goes on the next artifact build.")

	cmd.Flags().IntVar(&f.answers.Replicas, "replicas", 0, "Replica count (default 1).")
	cmd.Flags().Float64Var(&f.answers.CPU, "cpu", 0, "CPU per replica (default 0.5).")
	cmd.Flags().StringVar(&f.answers.Memory, "memory", "", "Memory per replica (default 512MB).")
	// Applied when the workload is created. `up` compares the artifact spec and
	// the runtime block, and importance is neither, so changing it for a
	// workload that already exists is an edit the file records and the next
	// deploy does not carry.
	cmd.Flags().StringVar(&f.answers.Importance, "importance", "",
		"Scheduling priority when capacity is constrained: "+
			strings.Join(manifest.ImportanceLevels, ", ")+
			" (default "+manifest.DefaultImportance+", or the bound workload's own level). Applied on create.")
}

func run(cmd *cobra.Command, f flags, format outputformat.OutputFormat) error {
	if err := checkDockerfileFlag(cmd, f.dockerfile); err != nil {
		return err
	}

	dir := f.dir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine the current directory: %w", err)
		}

		dir = cwd
	}

	// JSON output implies non-interactive: a wizard on stderr alongside a
	// machine-readable stdout is a trap for whoever is parsing it.
	yes := f.yes || viperx.GetBool("yes")

	result, err := wizard.Run(wizard.Options{
		Dir:            dir,
		NonInteractive: yes || format == outputformat.OutputFormatJSON,
		DryRun:         f.dryRun,
		Answers:        f.answers,
		Stderr:         cmd.ErrOrStderr(),
	})
	if err != nil {
		if errors.Is(err, wizard.ErrCancelled) {
			// Nothing was written, and the nonzero exit is what stops
			// `dr workload config && dr workload up`.
			return errors.New("cancelled: nothing was written")
		}

		return err
	}

	return render(cmd, f, format, result)
}

// checkDockerfileFlag holds --dockerfile to the one path a build can use.
// The flag exists so the answer is explicit rather than mysteriously
// ineffective, and so pointing it elsewhere fails here instead of at build
// time.
func checkDockerfileFlag(cmd *cobra.Command, path string) error {
	if !cmd.Flags().Changed("dockerfile") || path == wizard.DefaultDockerfilePath {
		return nil
	}

	if filepath.Clean(path) == wizard.DockerfileName {
		return nil
	}

	return fmt.Errorf("--dockerfile %q is not supported: builds use the %s at the project root. "+
		"Point --dir at the directory that holds it", path, wizard.DockerfileName)
}

func render(cmd *cobra.Command, f flags, format outputformat.OutputFormat, result wizard.Result) error {
	if format == outputformat.OutputFormatJSON {
		return outputformat.PrintJSONEnvelope(cmd.OutOrStdout(), "config", configResult{
			Path:              result.Path,
			Name:              result.Draft.Name,
			WorkloadID:        result.Draft.WorkloadID,
			CreateOnUp:        result.Draft.WorkloadID == "",
			BuildMode:         result.Draft.Build.Mode,
			Action:            result.Action,
			EnvKeysListed:     result.EnvKeysListed,
			EnvSecretsPending: result.EnvSecretsPending,
			EnvLiterals:       literalNames(result.Draft.EnvVars),
		})
	}

	stderr := cmd.ErrOrStderr()

	switch {
	case result.Action == wizard.ActionUnchanged:
		fmt.Fprintf(stderr, "%s already exists; edit it directly. Delete it to run setup again.\n", result.Path)
	case f.dryRun:
		fmt.Fprintf(stderr, "Dry run: %s was not written.\n\n", result.Path)
		fmt.Fprint(stderr, string(result.Content))
	default:
		fmt.Fprintf(stderr, "✓ Wrote %s\n", wizard.ShortPath(result.Path))

		if result.EnvKeysListed > 0 {
			fmt.Fprintf(stderr, "  %d %s from %s added%s.\n",
				result.EnvKeysListed, wizard.Plural(result.EnvKeysListed, "variable", "variables"),
				wizard.EnvFileName, secretSuffix(result.EnvSecretsPending))

			warnLiterals(stderr, result.Draft.EnvVars)
		}

		fmt.Fprint(stderr, "\nNext: dr workload up\n")
	}

	// stdout carries the path and nothing else, so it can be piped.
	fmt.Fprintln(cmd.OutOrStdout(), result.Path)

	return nil
}

// literalListLimit caps how many names are printed, because a long .env
// would bury the summary the list is there to qualify.
const literalListLimit = 8

// literalNames is the variables whose values the manifest carries in the
// clear. The classifier prefers to call a doubtful value secret, but it is
// still a heuristic: a short secret under an ordinary name reads as
// configuration, and its value goes into a file meant to be committed.
func literalNames(vars []manifest.EnvVar) []string {
	names := make([]string, 0, len(vars))

	for _, v := range vars {
		if !v.Secret {
			names = append(names, v.Name)
		}
	}

	return names
}

// warnLiterals names those variables. Nothing prompts on a headless run and
// the classifier's verdict is final there, so this listing is the only chance
// the user gets to catch a misread before the file is committed. Names only:
// the values are the thing being protected.
func warnLiterals(stderr io.Writer, vars []manifest.EnvVar) {
	names := literalNames(vars)
	if len(names) == 0 {
		return
	}

	shown := names
	suffix := ""

	if len(names) > literalListLimit {
		shown = names[:literalListLimit]
		suffix = fmt.Sprintf(" and %d more", len(names)-literalListLimit)
	}

	fmt.Fprintf(stderr, "  Values written in the clear: %s%s. Anything secret among them belongs in a %s reference instead.\n",
		strings.Join(shown, ", "), suffix, manifest.CredentialShorthandPrefix)
}

// secretSuffix names the entries that still need a credential id, because a
// bare count would read as "everything is ready to deploy".
func secretSuffix(secrets int) string {
	if secrets == 0 {
		return ""
	}

	return fmt.Sprintf("; %d %s a credential id in place of %s",
		secrets, wizard.Plural(secrets, "needs", "need"), manifest.CredentialPlaceholder)
}
