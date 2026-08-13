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

package wizard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// SetStdinTerminalForTest makes the wizard believe it has, or does not have,
// a terminal. It exists so a command-level test can prove that JSON output
// suppresses the wizard: under `go test` stdin is never a terminal, so
// without this the guard is unobservable and could be deleted unnoticed.
func SetStdinTerminalForTest(isTerminal bool) func() {
	original := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return isTerminal }

	return func() { isStdinTerminalFn = original }
}

// SetInteractiveFlowForTest replaces the wizard itself, so a command-level
// test can tell whether it would have run.
func SetInteractiveFlowForTest(flow func(Options, Detected) ([]byte, manifest.Draft, error)) func() {
	original := runInteractiveFlowFn
	runInteractiveFlowFn = flow

	return func() { runInteractiveFlowFn = original }
}

// Test seams: the wizard's tests replace these to keep the flow off the
// network and off the terminal.
var (
	getWorkloadFn        = workload.GetWorkloadDocument
	getArtifactFn        = workload.GetArtifactDocument
	resolveExecEnvFn     = workload.ResolveExecutionEnvironment
	isStdinTerminalFn    = reader.IsStdinTerminal
	runInteractiveFlowFn = interactiveFlowUnavailable
)

// Actions a run can report, mirroring what the JSON envelope carries.
const (
	// ActionCreated means the manifest was written.
	ActionCreated = "created"
	// ActionUnchanged means one was already there and nothing was touched.
	ActionUnchanged = "unchanged"
	// ActionPlanned means the run was a dry run: the content is what would
	// have been written, and nothing was. A caller keying on the action must
	// be able to tell that apart from a file that now exists.
	ActionPlanned = "planned"
)

// Options is one setup run.
type Options struct {
	// Dir is the project directory the manifest is written to.
	Dir string
	// NonInteractive answers everything from Answers and never prompts. It is
	// implied by the absence of a terminal and by JSON output.
	NonInteractive bool
	// DryRun renders the manifest and writes nothing.
	DryRun  bool
	Answers Answers
	// Stderr carries the wizard and its summary. Nothing the wizard says
	// belongs on stdout, which is the command's machine-readable channel.
	Stderr io.Writer
}

// Result is what a run settled.
type Result struct {
	// Path is the manifest, written or already present.
	Path string
	// Action is ActionCreated, ActionUnchanged or ActionPlanned.
	Action string
	// Content is the manifest as written, or as it would be written under
	// DryRun. Empty when the run changed nothing.
	Content []byte
	// EnvKeysListed is how many .env variables the written file carries, 0
	// when there was no .env or the user opted out, and EnvSecretsPending is
	// how many of those are commented out awaiting a credential. The command
	// reports both, so a headless run says what it actually did.
	EnvKeysListed     int
	EnvSecretsPending int
	// Draft is the answer set behind Content. For a bound workload it
	// describes the fields the wizard asked about, not the whole file.
	Draft manifest.Draft
}

// Run executes setup: it answers the questions from the flags, the project
// and, on a terminal, the user, then writes the manifest.
//
// An existing manifest ends the run untouched. That is the whole of setup's
// relationship with a configured project: the file is the interface, every
// later change is a hand edit, and deleting the file re-arms the wizard.
func Run(opts Options) (Result, error) {
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("cannot resolve %s: %w", opts.Dir, err)
	}

	path := manifest.Path(dir)
	if fsutil.FileExists(path) {
		return existing(path)
	}

	warnShadowedManifest(opts.Stderr, dir)

	detected := Detect(dir)

	if !opts.Answers.SkipEnv {
		warnUnreadEnvFile(opts.Stderr, detected)
	}

	content, draft, err := opts.resolve(detected)
	if err != nil {
		return Result{}, err
	}

	// A file that came from a running workload is judged as the platform's
	// news, not as a bug in the wizard.
	author := authorWizard
	if draft.WorkloadID != "" {
		author = authorLive
	}

	if err := checkRendered(content, dir, author); err != nil {
		return Result{}, err
	}

	action := ActionCreated
	if opts.DryRun {
		action = ActionPlanned
	}

	result := Result{
		Path:              path,
		Action:            action,
		Content:           content,
		Draft:             draft,
		EnvKeysListed:     len(draft.EnvVars),
		EnvSecretsPending: countSecrets(draft.EnvVars),
	}

	if opts.DryRun {
		return result, nil
	}

	if err := manifest.Write(path, content); err != nil {
		return Result{}, err
	}

	return result, nil
}

// countSecrets is how many entries are waiting on a credential.
func countSecrets(vars []manifest.EnvVar) int {
	pending := 0

	for _, v := range vars {
		if v.Secret {
			pending++
		}
	}

	return pending
}

// existing reports the manifest that is already there, and reads it rather
// than only noting its presence: the run's answer describes the project as it
// stands, not as it would have been configured. A file that cannot be read or
// does not validate is reported as the line-numbered error it is, because
// telling the user everything is fine and letting up discover otherwise is
// the worse of the two answers.
func existing(path string) (Result, error) {
	parsed, err := manifest.Load(path)
	if err != nil {
		return Result{}, err
	}

	if err := parsed.Validate(); err != nil {
		return Result{}, err
	}

	return Result{
		Path:   path,
		Action: ActionUnchanged,
		Draft: manifest.Draft{
			WorkloadID: parsed.WorkloadID(),
			Name:       parsed.Name(),
			Build:      manifest.Build{Mode: parsed.BuildMode()},
		},
	}, nil
}

// resolve produces the manifest bytes, from flags alone when nothing may
// prompt and from the wizard otherwise.
func (o Options) resolve(detected Detected) ([]byte, manifest.Draft, error) {
	if o.NonInteractive || !isStdinTerminalFn() {
		return o.resolveHeadless(detected)
	}

	return runInteractiveFlowFn(o, detected)
}

func (o Options) resolveHeadless(detected Detected) ([]byte, manifest.Draft, error) {
	if o.Answers.WorkloadID != "" {
		return o.resolveHeadlessBound(detected)
	}

	draft, err := o.Answers.draft(detected)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	content, err := draft.Render()
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	return content, draft, nil
}

// resolveHeadlessBound binds to a live workload without prompting: the live
// spec is downloaded and the flags are applied on top, so a headless bind is
// the same operation the picker performs.
func (o Options) resolveHeadlessBound(detected Detected) ([]byte, manifest.Draft, error) {
	live, err := fetchLive(o.Answers.WorkloadID)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	draft, err := o.Answers.applyTo(live.Defaults(), detected)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	// A name the workload already declares keeps its running value, so it is
	// not something this run added. Narrowing the draft here rather than
	// leaving it to Apply's own skip is what keeps the reported counts equal
	// to what reached the file.
	draft.EnvVars = live.NewEnvVars(draft.EnvVars)

	applied, err := live.Apply(draft)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	content, err := applied.Render()
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	return content, draft, nil
}

// applyTo layers the flags over defaults that already came from somewhere
// truthful, which for a bound workload is the workload itself. Only fields
// the user actually passed are overwritten, so binding keeps the live values
// for everything the command line did not mention.
func (a Answers) applyTo(defaults manifest.Draft, detected Detected) (manifest.Draft, error) {
	if err := a.check(); err != nil {
		return manifest.Draft{}, err
	}

	draft := defaults

	if a.Name != "" {
		draft.Name = a.Name
	}

	a.applyKind(&draft)
	a.applyReadiness(&draft)
	a.applySizing(&draft)
	a.applyEnv(&draft, detected)

	if err := a.checkA2A(draft.Type); err != nil {
		return manifest.Draft{}, err
	}

	if err := a.applyBuild(&draft, detected); err != nil {
		return manifest.Draft{}, err
	}

	return draft, nil
}

// applyKind layers the kind flags. Only a changed kind resets the flags
// belonging to the old one, and --a2a-enabled is a set-only flag: its absence
// is "not asked", not "off".
func (a Answers) applyKind(draft *manifest.Draft) {
	if a.Type != "" && a.Type != draft.Type {
		draft.Type = a.Type
		draft.A2AEnabled = false
	}

	if a.A2AEnabled {
		draft.A2AEnabled = true
	}
}

func (a Answers) applyReadiness(draft *manifest.Draft) {
	if a.Port != 0 {
		draft.Port = a.Port
	}

	if a.HealthPath != "" {
		draft.HealthPath = a.HealthPath
	}
}

// applyEnv layers the .env listing onto a bound workload. A live workload
// already carries its own environmentVars, so the placeholders are additive
// commentary and the opt-out still removes them.
func (a Answers) applyEnv(draft *manifest.Draft, detected Detected) {
	draft.EnvVars = a.envVars(detected)
}

// applySizing layers the runtime flags, which apply to a bound workload too.
// Anything left at zero keeps what the workload is already running with,
// rather than resetting it to the documented default.
func (a Answers) applySizing(draft *manifest.Draft) {
	if a.Replicas != 0 {
		draft.Runtime.Replicas = a.Replicas
	}

	if a.CPU != 0 {
		draft.Runtime.CPU = a.CPU
	}

	if a.Memory != "" {
		draft.Runtime.Memory = manifest.NormalizeMemory(a.Memory)
	}
}

// applyBuild layers the image-source flags. Staying on the mode the workload
// already uses is an edit to that build, so only the fields actually passed
// change and the rest keep their live values: --entrypoint alone changes a
// generated build's command without demanding its environment be repeated.
// Switching modes is the other thing entirely, and there the mode's companion
// flags are required, because nothing in the old build answers for the new one.
func (a Answers) applyBuild(draft *manifest.Draft, detected Detected) error {
	mode := a.buildMode(detected)
	if !a.explicitBuild() || mode == "" {
		return nil
	}

	if mode == draft.Build.Mode {
		return a.mergeBuild(&draft.Build)
	}

	build, err := a.build(detected)
	if err != nil {
		return err
	}

	draft.Build = build

	return nil
}

// mergeBuild edits a build already in the requested mode, field by field.
func (a Answers) mergeBuild(build *manifest.Build) error {
	switch build.Mode {
	case manifest.BuildModeImage:
		if a.Image != "" {
			build.ImageURI = a.Image
		}

	case manifest.BuildModeGenerated:
		if a.Entrypoint != "" {
			entrypoint, err := splitCommand(a.Entrypoint)
			if err != nil {
				return fmt.Errorf("--entrypoint %q: %w", a.Entrypoint, err)
			}

			build.Entrypoint = entrypoint
		}

		if a.ExecutionEnvironment != "" {
			id, versionID, err := resolveExecEnvFn(a.ExecutionEnvironment)
			if err != nil {
				return err
			}

			build.ExecutionEnvironmentID, build.ExecutionEnvironmentVersionID = id, versionID
		}

	case manifest.BuildModeDockerfile:
		// The mode is the whole answer; there is nothing else to carry.
	}

	return nil
}

// explicitBuild reports whether the flags actually asked for an image source,
// as opposed to the detector noticing a Dockerfile. Binding must not switch a
// running workload's build mode just because the checkout happens to have a
// Dockerfile in it.
func (a Answers) explicitBuild() bool {
	return a.BuildMode != "" || a.Image != "" || a.ExecutionEnvironment != "" || a.Entrypoint != ""
}

// fetchLive downloads a workload and the artifact it runs. The workload is
// fetched by id, so a typo fails here, at setup, rather than at the first
// deploy.
func fetchLive(workloadID string) (manifest.Live, error) {
	workloadDoc, err := getWorkloadFn(workloadID)
	if err != nil {
		return manifest.Live{}, fmt.Errorf("cannot bind to workload %s: %w", workloadID, err)
	}

	artifactID := workloadDoc.String("artifactId")
	if artifactID == "" {
		return manifest.Live{}, fmt.Errorf("workload %s has no artifact to copy; create a new workload instead", workloadID)
	}

	artifactDoc, err := getArtifactFn(artifactID)
	if err != nil {
		return manifest.Live{}, fmt.Errorf("cannot read the artifact of workload %s: %w", workloadID, err)
	}

	return manifest.NewLive(workloadID, workloadDoc, artifactDoc), nil
}

// checkRendered parses and validates what is about to be written, so a file
// this CLI would refuse to read is never left on disk.
//
// The wording matters, because the content has three possible authors. A file
// the wizard composed from its own answers failing here is a bug in the
// wizard. A file that came from a running workload failing here means the
// workload uses something the reader has not caught up with, which is the
// platform's news, not the user's mistake. And a file the user just edited is
// simply theirs to fix.
func checkRendered(content []byte, dir string, author contentAuthor) error {
	parsed, err := manifest.Parse(content, dir)
	if err == nil {
		err = parsed.Validate()
	}

	if err == nil {
		return nil
	}

	switch author {
	case authorWizard:
		return fmt.Errorf("the generated manifest is not valid, which is a bug: %w", err)
	case authorLive:
		return fmt.Errorf("workload %s uses settings this CLI cannot yet write to a manifest.\n"+
			"Nothing was written. Please report this with the detail below:\n%w", manifestName(parsed), err)
	case authorUser:
		// The user just edited it, so the finding speaks for itself.
		return err
	}

	return err
}

// contentAuthor says who wrote the bytes being checked, which decides whose
// problem a failure is.
type contentAuthor int

const (
	// authorWizard is the wizard's own rendering of its own answers.
	authorWizard contentAuthor = iota
	// authorLive came from a running workload.
	authorLive
	// authorUser came from the user's editor.
	authorUser
)

func manifestName(parsed *manifest.Manifest) string {
	if parsed == nil || parsed.Name() == "" {
		return "this"
	}

	return parsed.Name()
}

// warnShadowedManifest names an ancestor manifest rather than letting two
// files quietly shadow each other: up searches upward, so a nested file
// changes which one a deploy from the parent directory reads.
func warnShadowedManifest(stderr io.Writer, dir string) {
	if stderr == nil {
		return
	}

	parent := filepath.Dir(dir)
	if parent == dir {
		return
	}

	found, err := manifest.Locate(parent)
	if err != nil {
		if !errors.Is(err, manifest.ErrNotFound) {
			fmt.Fprintf(stderr, "Warning: cannot check for a manifest above %s: %v\n", dir, err)
		}

		return
	}

	fmt.Fprintf(stderr,
		"Warning: a manifest already exists at %s. Writing one in %s means a deploy from either directory reads a different file.\n",
		found, dir)
}

// warnUnreadEnvFile says so when a .env is there but contributed nothing.
// Setup continues, because the import is a convenience and the manifest is
// valid without it, but it cannot pass in silence: the whole reason the file
// is read is that `up` never reads it, so an import that quietly did not
// happen surfaces as a container missing its configuration at runtime.
func warnUnreadEnvFile(stderr io.Writer, detected Detected) {
	if stderr == nil || detected.EnvErr == nil {
		return
	}

	fmt.Fprintf(stderr,
		"Warning: %v. Nothing from it was imported; add the variables to %s by hand.\n",
		detected.EnvErr, manifest.FileName)
}

// ShortPath names a path relative to the working directory when it is under
// it, which is how the user would say it themselves. Exported so the command
// and the screens render the same path the same way.
func ShortPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}

	relative, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return path
	}

	return relative
}

// interactiveFlowUnavailable stands in until the wizard screens land. The
// command is behind a feature gate, so the only way to reach this is to run
// the gated command on a terminal without flags.
func interactiveFlowUnavailable(_ Options, _ Detected) ([]byte, manifest.Draft, error) {
	const msg = "interactive setup is not available yet; " +
		"pass --yes with the flags that answer each question"

	return nil, manifest.Draft{}, errors.New(msg)
}
