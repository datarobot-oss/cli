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
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/cmd/workload/internal/envconfirm"
	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
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
	// Action is created, updated, unchanged or planned. A caller keying on it
	// has to be able to tell a dry run's planned from a file that now exists,
	// and a --sync-env edit from the run that wrote the file.
	Action string `json:"action"`
	// EnvKeysListed is how many .env variables this run put in the file and
	// EnvSecretsPending how many of those still name the placeholder instead
	// of a credential id. Without them a pipeline reading only this envelope
	// would see a created manifest and no sign that it cannot deploy yet. A
	// create writes the whole list, so there the count is also what the file
	// carries; a --sync-env run adds to a list already there, so there it
	// is only what it added.
	EnvKeysListed int `json:"envKeysListed"`
	// EnvNamesRemoved is how many entries the reconciliation took out because
	// .env no longer names them, reported apart from the rest because it is
	// the one act that loses configuration.
	EnvNamesRemoved   int `json:"envNamesRemoved"`
	EnvSecretsPending int `json:"envSecretsPending"`
	// EnvValuesUpdated is how many declared literals a --sync-env run
	// rewrote, and EnvSecretsRotated how many credentials it re-sent. Counted
	// apart from the added variables because they are different acts: one adds
	// a name, the other changes what a name is worth.
	EnvValuesUpdated  int `json:"envValuesUpdated"`
	EnvSecretsRotated int `json:"envSecretsRotated"`
	// EnvSecretsNotRotated is how many re-sends failed. The terminal names
	// each one, and this envelope is what a JSON run has instead: a failed
	// re-send leaves the old value serving, changes no file and stops nothing,
	// so a pipeline with no count for it cannot tell that run from a clean one.
	EnvSecretsNotRotated int `json:"envSecretsNotRotated"`
	// EnvLiterals names the variables this run wrote into the file in the
	// clear, so an audit step can check them without parsing the YAML. A
	// create writes the whole list, so there it is every literal the file
	// carries; a --sync-env run touches part of one, so
	// there it is only what that run put there.
	EnvLiterals []string `json:"envLiterals"`
}

// flags is the flag set, held together so the run path reads as one answer
// set rather than eighteen lookups.
type flags struct {
	dir        string
	dockerfile string
	dryRun     bool
	yes        bool
	syncEnv    bool
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

The exception is --sync-env. It brings the manifest into line with .env in one
act: a name the file does not declare is added, a literal whose value has moved
is rewritten, and the credential behind a secret is re-sent, and a name it no
longer carries is taken out. It prints a table of what it would do to every
variable, and why, and asks before it writes a line or sends a value, because
.env winning on everything means a stale copy is enough to delete something a
running workload needs. It is opt-in because .env is
your local copy and is allowed to drift: a deploy that silently picked up
whatever it grew since is what this command is built not to do.

A secret is re-sent without being compared. The platform never returns a
stored value, so nothing can tell a rotated key from an untouched one, and a
run that reported "no change" for a key you had just rotated would be worse
than one request too many.

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
    --entrypoint "uvicorn app:app --host 0.0.0.0 --port 8080"
  dr workload config --sync-env`,
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
	_ = viperx.BindEnv(cli.YesFlagName, "DATAROBOT_CLI_NON_INTERACTIVE")

	telemetry.TrackWith(cmd, func(cmd *cobra.Command, _ []string) map[string]any {
		nonInteractive := cli.IsNonInteractive(cmd)

		return map[string]any{
			"yes":           nonInteractive,
			"type":          f.answers.Type,
			"build_mode":    f.answers.BuildMode,
			"bound":         f.answers.WorkloadID != "",
			"sync_env":      f.syncEnv,
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
	cmd.Flags().BoolVarP(&f.yes, cli.YesFlagName, "y", false,
		"Do not prompt; answer every question from flags and what the project directory shows. "+
			"A question neither of those answers is an error naming the flag that would settle it.")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print the manifest and write nothing.")
	cmd.Flags().BoolVar(&f.answers.SkipEnv, "skip-env", false,
		"Do not carry the project's .env into the manifest. By default its variables are written there: "+
			"ordinary values as literals, secrets as credential references you complete later.")
	// The one flag that acts on a manifest already present, and the reason it
	// is opt-in: re-reading .env on every run would carry whatever the file
	// grew since into a deploy nobody asked to change. Exclusive with
	// --skip-env, which says the opposite about the same file.
	cmd.Flags().BoolVar(&f.syncEnv, "sync-env", false,
		"Bring the manifest into line with .env: add the variables it does not declare, rewrite a literal "+
			"whose value has moved, and re-send the credential behind a secret. Prints what it would do and "+
			"asks first, removals included: a name dropped from .env is taken out. Secrets are re-sent "+
			"without being compared, because the platform never returns a stored value.")

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
		"Readiness path, e.g. /health. A new manifest gets no probe without it: a probe has to be written "+
			"for the app it probes, and a wrong one stops a healthy deploy from ever going ready. "+
			"With --workload-id the bound workload's own probe is kept.")
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

	if err := checkSyncEnvFlags(cmd, f); err != nil {
		return err
	}

	// Shared with `up` and `delete` so one typo is refused one way. Without it
	// this command walked into a directory that is not there and reported the
	// missing Dockerfile instead, three messages after the one fact that
	// explains them.
	dir, err := idargs.ProjectDir(f.dir)
	if err != nil {
		return err
	}

	// JSON output implies non-interactive: a wizard on stderr alongside a
	// machine-readable stdout is a trap for whoever is parsing it.
	yes := cli.IsNonInteractive(cmd)

	asJSON := format == outputformat.OutputFormatJSON

	// No writer at all under JSON, rather than a guard inside each of the
	// wizard's reporters: stdout purity is only half the contract, and one of
	// eight reporters forgetting the check is all it takes to break
	// `2>&1 | jq .`. What a terminal run would have warned about becomes an
	// error there instead, so nothing is quietly lost.
	stderr := cmd.ErrOrStderr()
	if asJSON {
		stderr = nil
	}

	result, err := wizard.Run(wizard.Options{
		Dir:            dir,
		NonInteractive: yes || asJSON,
		DryRun:         f.dryRun,
		SyncEnv:        f.syncEnv,
		// f.yes rather than the merged signal: the environment variable that
		// suppresses wizards in CI is not consent to overwrite a value on the
		// tenant, which is the line `dr workload delete` already draws.
		Confirm: envconfirm.Ask(cmd.ErrOrStderr(), cmd.InOrStdin(), envconfirm.Policy{
			// The flag or the environment variable, matching what the rest of
			// this command treats as an answer. What is not an answer is the
			// mere absence of a terminal, which is the case this refuses.
			Yes:         yes,
			DryRun:      f.dryRun,
			Interactive: !asJSON && idargs.CanAsk(cmd),
			Silent:      asJSON,
		}),
		JSONOutput: asJSON,
		Answers:    f.answers,
		// With the --dir this run was given: this command looks only where it
		// is pointed, so a remedy without it is one the reader cannot run from
		// where they are standing.
		Remedy: "dr workload config" + manifest.DirFlag(dir),
		Stderr: stderr,
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

// editsEnv reports that this run acts on the .env of a manifest that already
// exists, which is the question four sites here and one in the wizard were
// each spelling out for themselves.
func (f flags) editsEnv() bool {
	return f.syncEnv
}

// setupOnlyFlags are the answers that only a run writing a manifest can act
// on. An import edits one key of a file that already answered all of them.
var setupOnlyFlags = []string{
	"workload-id", "name", "type", "a2a-enabled", "build-mode", "dockerfile",
	"execution-environment", "entrypoint", "image", "port", "health",
	"no-readiness-probe", "replicas", "cpu", "memory", "importance",
}

// checkSyncEnvFlags refuses the combinations --sync-env cannot honour,
// rather than accepting them and reporting a success that did none of it.
//
// Silently dropping them is the trap worth closing: a CI line that gains
// --sync-env alongside its existing --workload-id would stop binding the
// workload and still exit 0.
func checkSyncEnvFlags(cmd *cobra.Command, f flags) error {
	if !f.editsEnv() {
		return nil
	}

	if f.answers.SkipEnv {
		return errors.New("--sync-env reads .env, and --skip-env says not to read it at all: " +
			"pass one or the other")
	}

	ignored := make([]string, 0, len(setupOnlyFlags))

	for _, name := range setupOnlyFlags {
		if cmd.Flags().Changed(name) {
			ignored = append(ignored, "--"+name)
		}
	}

	if len(ignored) > 0 {
		return fmt.Errorf("--sync-env acts on the .env variables of a manifest that already "+
			"exists, so they cannot also apply %s. Edit %s directly for those, and run the env flags on their own",
			strings.Join(ignored, ", "), manifest.FileName)
	}

	return nil
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
			Path:                 result.Path,
			Name:                 result.Draft.Name,
			WorkloadID:           result.Draft.WorkloadID,
			CreateOnUp:           result.Draft.WorkloadID == "",
			BuildMode:            result.Draft.Build.Mode,
			Action:               result.Action,
			EnvKeysListed:        result.EnvKeysListed,
			EnvNamesRemoved:      result.EnvNamesRemoved,
			EnvSecretsPending:    result.EnvSecretsPending,
			EnvValuesUpdated:     result.EnvValuesUpdated,
			EnvSecretsRotated:    result.EnvSecretsRotated,
			EnvSecretsNotRotated: result.EnvSecretsNotRotated,
			EnvLiterals:          wizard.LiteralNames(result.Draft.EnvVars),
		})
	}

	stderr := cmd.ErrOrStderr()

	switch {
	case result.Action == wizard.ActionUnchanged && result.EnvSecretsRotated > 0:
		// A run that only re-sent secrets. The wizard has already said what
		// went, and what is left to say is where: the credential store took
		// the value and the file carrying the reference is byte for byte what
		// it was, so "✓ Updated .datarobot.yaml" would be a plain untruth.
		fmt.Fprintf(stderr, "%s was not changed: a secret's value lives in the credential store, not in the file.\n",
			wizard.ShortPath(result.Path))

		// A container reads its credentials when it starts, so one already
		// running serves the value it started with until it is replaced. `up`
		// will not do it: the manifest it compares against is the one it
		// already deployed, and a rotation does not touch that, so it reports
		// the workload as up to date and changes nothing.
		dir := manifest.DirFlag(filepath.Dir(result.Path))
		fmt.Fprintf(stderr,
			"\nA running container keeps the value it started with. To serve the new one:\n"+
				"  dr workload stop --yes%s\n  dr workload start --yes%s\n", dir, dir)
	case result.Action == wizard.ActionUnchanged:
		// An import that found nothing already said why on its way through:
		// the file was complete, or absent, or complete only because what is
		// missing was held back, and only the wizard can tell those apart.
		// Repeating a guess here would contradict it, and "delete it to run
		// setup again" is advice for a question that was not asked.
		if !f.editsEnv() {
			fmt.Fprintf(stderr, "%s already exists; edit it directly. Delete it to run setup again.\n", result.Path)
		}
	case f.dryRun && f.editsEnv():
		fmt.Fprintf(stderr, "Dry run: %s was not written.\n", result.Path)
		reportEnvAdded(stderr, result)
	case f.dryRun:
		fmt.Fprintf(stderr, "Dry run: %s was not written.\n\n", result.Path)
		fmt.Fprint(stderr, string(result.Content))
	default:
		fmt.Fprintf(stderr, "✓ %s %s\n", writeVerb(result.Action), wizard.ShortPath(result.Path))

		reportEnvAdded(stderr, result)

		// The wizard may have written the manifest into a directory the shell
		// is not standing in; a bare `up` there would configure and deploy
		// the wrong tree.
		fmt.Fprintf(stderr, "\nNext: dr workload up%s\n", manifest.DirFlag(filepath.Dir(result.Path)))
	}

	// stdout carries the path and nothing else, so it can be piped.
	fmt.Fprintln(cmd.OutOrStdout(), result.Path)

	return nil
}

// writeVerb says what happened to the file. Updated rather than wrote, because
// the file was the user's before an edit and every line it already had is
// still theirs.
func writeVerb(action string) string {
	if action == wizard.ActionUpdated {
		return "Updated"
	}

	return "Wrote"
}

// reportEnvAdded says what this run put in the file, and names the values it
// wrote in the clear. Shared by the write and the import's dry run, which
// report the same act and must describe it the same way.
func reportEnvAdded(stderr io.Writer, result wizard.Result) {
	if result.EnvValuesUpdated > 0 {
		fmt.Fprintf(stderr, "  %d declared %s updated to match %s.\n",
			result.EnvValuesUpdated, wizard.Plural(result.EnvValuesUpdated, "value", "values"),
			wizard.EnvFileName)
	}

	if result.EnvKeysListed > 0 {
		fmt.Fprintf(stderr, "  %d %s from %s added%s.\n",
			result.EnvKeysListed, wizard.Plural(result.EnvKeysListed, "variable", "variables"),
			wizard.EnvFileName, secretSuffix(result.EnvSecretsPending))
	}

	// Said out loud, because it is the one act that loses configuration rather
	// than gaining it, and the two counts above would otherwise report a run
	// that added three variables and dropped four as a run that added three.
	if result.EnvNamesRemoved > 0 {
		fmt.Fprintf(stderr, "  %d %s removed, gone from %s.\n",
			result.EnvNamesRemoved, wizard.Plural(result.EnvNamesRemoved, "variable", "variables"),
			wizard.EnvFileName)
	}

	// Last, and outside every count: every value this run put in the file is
	// owed the same disclosure, whether it arrived as a new name or as a new
	// value for one already there.
	wizard.WarnLiterals(stderr, result.Draft.EnvVars)
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
