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
// test can tell whether it would have run. The string is the project
// directory the flow settled on, which is Detected.Dir unless the user chose
// another on the directory screen.
func SetInteractiveFlowForTest(flow func(Options, Detected) ([]byte, manifest.Draft, string, error)) func() {
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
	runInteractiveFlowFn = runInteractiveFlow
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
	// ActionUpdated means a manifest that was already there was edited in
	// place. It is deliberately not ActionCreated: a pipeline that keys on
	// the action has to be able to tell a file it now owns from one it
	// added to.
	ActionUpdated = "updated"
)

// Options is one setup run.
type Options struct {
	// Dir is the project directory the manifest is written to.
	Dir string
	// NonInteractive answers everything from Answers and never prompts. It is
	// implied by the absence of a terminal and by JSON output.
	NonInteractive bool
	// DryRun renders the manifest and writes nothing.
	DryRun bool
	// ImportEnv re-reads .env over a manifest that already exists and adds
	// the variables it does not declare. It is the one answer that survives
	// the existing-manifest guard, and it has to be asked for: setup stays a
	// one-time act, and a deploy that quietly picked up whatever .env grew
	// since is the behaviour this command was built not to have.
	ImportEnv bool
	// JSONOutput says the run's answer is a machine-readable envelope. Under
	// it the command hands the wizard no Stderr at all, because stdout purity
	// is only half the contract and `2>&1 | jq .` has to parse too; what a
	// terminal run would have warned about becomes an error instead, so the
	// signal is carried by the exit code rather than dropped.
	JSONOutput bool
	Answers    Answers
	// Stderr carries the wizard and its summary. Nothing the wizard says
	// belongs on stdout, which is the command's machine-readable channel.
	Stderr io.Writer
}

// Result is what a run settled.
type Result struct {
	// Path is the manifest, written or already present.
	Path string
	// Action is ActionCreated, ActionUpdated, ActionUnchanged or ActionPlanned.
	Action string
	// Content is the manifest as written, or as it would be written under
	// DryRun. Empty when the run changed nothing.
	Content []byte
	// EnvKeysListed is how many .env variables this run put in the file, 0
	// when there was no .env or the user opted out, and EnvSecretsPending is
	// how many of those still await a credential. The command reports both, so
	// a headless run says what it actually did. A create writes the whole
	// list, so for it the count is also what the file carries; an import adds
	// to a list already there, so for it the count is only what it added.
	EnvKeysListed     int
	EnvSecretsPending int
	// Draft is the answer set behind Content. For a bound workload it
	// describes the fields the wizard asked about, not the whole file.
	Draft manifest.Draft
}

// Run executes setup: it answers the questions from the flags, the project
// and, on a terminal, the user, then writes the manifest.
//
// An existing manifest ends the run untouched unless Options.ImportEnv asks
// for the one edit setup makes to a configured project. That is the whole of
// setup's relationship with one: the file is the interface, every other change
// is a hand edit, and deleting the file re-arms the wizard.
func Run(opts Options) (Result, error) {
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("cannot resolve %s: %w", opts.Dir, err)
	}

	path := manifest.Path(dir)
	if fsutil.FileExists(path) {
		return opts.configured(path, dir)
	}

	if err := opts.checkNothingToImport(dir); err != nil {
		return Result{}, err
	}

	return opts.create(dir)
}

// create is setup proper: the run that answers the questions and writes a
// manifest for a project that has none.
func (o Options) create(dir string) (Result, error) {
	warnShadowedManifest(o.Stderr, dir)

	detected := Detect(dir)

	if err := o.checkEnvFile(detected); err != nil {
		return Result{}, err
	}

	content, draft, projectDir, err := o.resolve(detected)
	if err != nil {
		return Result{}, err
	}

	// The interactive flow may have moved the project: everything from here
	// on — the write, the validation's Dockerfile check, the reported path —
	// belongs to the directory resolve settled on, which headless runs return
	// unchanged.
	path := manifest.Path(projectDir)

	// The chosen directory's own .env gets the same courtesy the starting
	// one got above: a parse failure must not read as "there was nothing to
	// import". The flow re-detected on the way, but only the warning's
	// reader was lost — the TUI owned the terminal at the time.
	if projectDir != dir {
		if err := o.checkEnvFile(Detect(projectDir)); err != nil {
			return Result{}, err
		}
	}

	// A file that came from a running workload is judged as the platform's
	// news, not as a bug in the wizard.
	author := authorWizard
	if draft.WorkloadID != "" {
		author = authorLive
	}

	if err := checkRendered(content, projectDir, author); err != nil {
		return Result{}, err
	}

	action := ActionCreated
	if o.DryRun {
		action = ActionPlanned
	}

	result := Result{
		Path:              path,
		Action:            action,
		Content:           content,
		Draft:             draft,
		EnvKeysListed:     len(draft.EnvVars),
		EnvSecretsPending: pendingSecrets(draft.EnvVars),
	}

	if o.DryRun {
		return result, nil
	}

	if err := manifest.Write(path, content); err != nil {
		return Result{}, err
	}

	return result, nil
}

// checkNothingToImport refuses --import-env in a directory with no manifest.
// The flag names an existing file to add to, and falling through to setup
// would create one instead, in a directory the flag says was already
// configured, and mint credentials for the whole .env on the way.
func (o Options) checkNothingToImport(dir string) error {
	if !o.ImportEnv {
		return nil
	}

	// `up` reads the nearest manifest at or above its directory, so a project
	// laid out with the file at the repository root has one governing this
	// directory that this command, which only ever looks in the directory it
	// was given, would otherwise deny the existence of.
	if above, err := manifest.Locate(filepath.Dir(dir)); err == nil {
		return fmt.Errorf(
			"no %s in %s, so there is nothing to import into. The one at %s is what a deploy from here reads: "+
				"run this with --dir %s",
			manifest.FileName, dir, above, filepath.Dir(above))
	}

	return fmt.Errorf(
		"no %s in %s, so there is nothing to import into. "+
			"Run 'dr workload config' to create one, or point --dir at the project that has one",
		manifest.FileName, dir)
}

// configured handles a project that has been set up already: the one edit that
// was asked for, or the file itself, reported and untouched.
//
// Both halves start from the file rather than from the answers, which is the
// difference between this and a run that writes one. Setup answers a project
// that has none; here the manifest is the truth and the most a run may do is
// add to it.
func (o Options) configured(path, dir string) (Result, error) {
	parsed, err := manifest.Load(path)
	if err != nil {
		return Result{}, err
	}

	if err := parsed.Validate(); err != nil {
		return Result{}, err
	}

	detected := Detect(dir)

	// Both halves read the same .env, so both owe the same account of one that
	// could not be read. Without it the drift notice's silence, which means
	// "nothing missing", would also be what a file that failed to parse looks
	// like.
	if err := o.checkEnvFile(detected); err != nil {
		return Result{}, err
	}

	if o.ImportEnv {
		return o.editEnv(path, parsed, detected)
	}

	return o.existing(path, parsed, detected)
}

// existing reports the manifest that is already there, and reads it rather
// than only noting its presence: the run's answer describes the project as it
// stands, not as it would have been configured. A file that cannot be read or
// does not validate is reported as the line-numbered error it is, because
// telling the user everything is fine and letting up discover otherwise is
// the worse of the two answers.
//
// It changes nothing, and detected is here only so the run can say when .env
// has grown past the file. That notice is the point: the import is one-time by
// design, so the drift it leaves behind is invisible until a container fails
// at first use, and this is the one command in a position to mention it.
func (o Options) existing(path string, parsed *manifest.Manifest, detected Detected) (Result, error) {
	o.warnEnvDrift(parsed, detected)

	return Result{
		Path:   path,
		Action: ActionUnchanged,
		Draft:  draftOf(parsed),
	}, nil
}

// warnEnvDrift names the .env variables the manifest does not declare, and the
// flag that adds them.
//
// Names only, never values. The deploy plan redacts environment values on
// purpose, and a warning that pasted a freshly added API key onto the terminal
// would be the one place that undoes it.
//
// Silent where there is no .env, which is the ordinary CI case, and silent
// under --skip-env, which is the user having already said the file is not to
// be read.
func (o Options) warnEnvDrift(parsed *manifest.Manifest, detected Detected) {
	if o.quiet() {
		return
	}

	// Through envVars rather than detected.EnvVars directly, so the notice
	// counts what --import-env would actually add: SkipEnv silences it, and a
	// variable the classifier called local-only is held back on purpose, so it
	// is reported separately below rather than as something the flag would fix.
	fresh := undeclared(o.Answers.envVars(detected), parsed.EnvVarNames())
	if len(fresh) == 0 {
		o.warnEnvHeldBack(parsed, detected)

		return
	}

	// Naming a flag that cannot run on this file would be an errand, not
	// advice. The refusal says what the shape is and what to do instead.
	if err := parsed.CanDeclareEnvVars(); err != nil {
		fmt.Fprintf(o.Stderr,
			"%s defines %d %s the manifest does not declare, and %s cannot be added automatically: %v\n",
			EnvFileName, len(fresh), Plural(len(fresh), "variable", "variables"),
			Plural(len(fresh), "it", "they"), err)

		return
	}

	missing := make([]string, 0, len(fresh))
	for _, v := range fresh {
		missing = append(missing, v.Name)
	}

	fmt.Fprintf(o.Stderr,
		"%s defines %d %s the manifest does not declare, so %s would not reach the container: %s.\n"+
			"  Add %s with 'dr workload config --import-env'.\n",
		EnvFileName, len(missing), Plural(len(missing), "variable", "variables"),
		Plural(len(missing), "it", "they"), JoinNames(missing),
		Plural(len(missing), "it", "them"))

	// What the flag would not add is news whether or not it has something to
	// add: a name held back is held back either way, and only saying so when
	// the drift list happens to be empty makes it a matter of luck.
	o.warnEnvHeldBack(parsed, detected)
}

// warnEnvHeldBack covers the two ways a manifest can declare every name .env
// offers and still not be finished.
//
// A local-only variable is a classifier verdict, not a fact, and this path has
// no table to overrule it on, so the name has to be said or the user has no
// way to learn it was held back. A placeholder is an entry the CLI itself
// wrote and never completed, and because the name counts as declared, every
// later import skips it: without this line the retry after the credential
// store comes back reports "nothing to add" about a file a deploy refuses.
func (o Options) warnEnvHeldBack(parsed *manifest.Manifest, detected Detected) {
	if o.quiet() || o.Answers.SkipEnv {
		return
	}

	var local []string

	declared := parsed.EnvVarNames()

	for _, v := range detected.EnvVars {
		if v.Kind == EnvLocal && !declared[v.Name] {
			local = append(local, v.Name)
		}
	}

	if len(local) > 0 {
		fmt.Fprintf(o.Stderr,
			"%s defines %s that %s read as local-only, so %s deliberately left out of the manifest: %s.\n"+
				"  Add %s by hand if the workload needs %s.\n",
			EnvFileName, Plural(len(local), "a variable", "variables"),
			Plural(len(local), "was", "were"), Plural(len(local), "it is", "they are"),
			JoinNames(local),
			Plural(len(local), "it", "them"), Plural(len(local), "it", "them"))
	}

	if pending := parsed.PendingEnvNames(); len(pending) > 0 {
		fmt.Fprintf(o.Stderr,
			"%s still names %s for %s, so a deploy will refuse %s: %s.\n"+
				"  Store the %s as a credential and put its id in the file.\n",
			manifest.FileName, manifest.CredentialPlaceholder,
			Plural(len(pending), "a variable", "variables"),
			Plural(len(pending), "it", "them"), JoinNames(pending),
			Plural(len(pending), "value", "values"))
	}
}

func (o Options) editEnv(path string, parsed *manifest.Manifest, detected Detected) (Result, error) {
	// Asked before anything is stored, because storing is the step that cannot
	// be undone. A credential is created on the tenant for good, and finding
	// out at the write that this file was never going to accept the reference
	// leaves a secret behind that nothing points at and whose name every retry
	// then collides with.
	//
	// configured has already refused a .env this could not read, so what
	// reaches here is a parsed file and a parsed environment.
	if err := parsed.CanDeclareEnvVars(); err != nil {
		return Result{}, err
	}

	wanted := o.Answers.envVars(detected)

	result := Result{Path: path, Action: ActionUnchanged, Draft: draftOf(parsed)}

	if err := o.addEnv(path, parsed, detected, wanted, &result); err != nil {
		return Result{}, err
	}

	if result.Action == ActionUnchanged {
		o.reportNothingToImport(parsed, detected)

		return result, nil
	}

	// What the flags would not touch is news whether or not they touched
	// something: a name held back is held back either way, and a placeholder
	// left by an earlier run is still what the next deploy refuses.
	o.warnEnvHeldBack(parsed, detected)

	return result, nil
}

// addEnv is the additive half: the names the manifest does not carry yet.
func (o Options) addEnv(
	path string, parsed *manifest.Manifest, detected Detected, wanted []manifest.EnvVar, result *Result,
) error {
	fresh := undeclared(wanted, parsed.EnvVarNames())
	if len(fresh) == 0 {
		return nil
	}

	// The guard above reads the tree the manifest was parsed into, which
	// cannot see what only the file says: a second YAML document after the
	// first, which the reader skips and the edit refuses. So the edit is run
	// once as a preview and thrown away. It is the only guard that cannot
	// disagree with the write it guards, and it costs a parse to save a
	// credential nothing would ever point at.
	if _, _, err := manifest.ImportEnvVars(path, fresh, true); err != nil {
		return err
	}

	fresh = o.storeSecrets(fresh, detected, parsed.Name())

	added, content, err := manifest.ImportEnvVars(path, fresh, o.DryRun)
	if err != nil {
		return err
	}

	// The action comes from what the edit did, not from what was asked for.
	// ImportEnvVars reports nothing added when the file changed under the run
	// and already declares these names, and calling that an update would tell
	// a pipeline a file moved that never did.
	if len(added) == 0 {
		return nil
	}

	if len(content) > 0 {
		result.Content = content
	}

	result.Action = o.editAction()
	result.EnvKeysListed = len(added)

	// Not counted on a dry run. Nothing was stored, so every secret still
	// carries the placeholder, and reporting that as "needs a credential id"
	// would describe the preview rather than the run it previews.
	if !o.DryRun {
		result.EnvSecretsPending = pendingSecrets(added)
	}

	// The variables this run added, so the command can name the ones whose
	// values it just wrote out in the clear. Without them the only disclosure
	// the classifier's verdict ever gets is skipped on the one path that adds
	// fresh values to a file headed for git.
	result.Draft.EnvVars = append(result.Draft.EnvVars, added...)

	return nil
}

// editAction is what an edit that did something reports.
func (o Options) editAction() string {
	if o.DryRun {
		return ActionPlanned
	}

	return ActionUpdated
}

// reportNothingToImport says why an edit changed nothing. The command prints
// no line of its own for this case, because only here is it known whether the
// file was complete, absent, or complete only because the names that are
// missing were held back.
func (o Options) reportNothingToImport(parsed *manifest.Manifest, detected Detected) {
	if o.quiet() {
		return
	}

	dir := filepath.Dir(parsed.Path)

	// Asked of the filesystem rather than of the parse: a .env holding only
	// comments yields no variables and is still a file the user is looking at,
	// and telling them it does not exist points them at the wrong fix.
	if !fsutil.FileExists(filepath.Join(dir, EnvFileName)) {
		fmt.Fprintf(o.Stderr, "No %s in %s, so there is nothing to import.\n", EnvFileName, dir)

		return
	}

	fmt.Fprintf(o.Stderr, "%s already declares every variable in %s that can be imported.\n",
		ShortPath(parsed.Path), EnvFileName)

	o.warnEnvHeldBack(parsed, detected)
}

// undeclared is the variables whose names the manifest does not already
// carry, in the order .env defines them.
func undeclared(vars []manifest.EnvVar, declared map[string]bool) []manifest.EnvVar {
	fresh := make([]manifest.EnvVar, 0, len(vars))

	for _, v := range vars {
		if !declared[v.Name] {
			fresh = append(fresh, v)
		}
	}

	return fresh
}

// draftOf is what a run over an existing manifest can say about it: the fields
// the file answers, not the whole spec.
func draftOf(parsed *manifest.Manifest) manifest.Draft {
	return manifest.Draft{
		WorkloadID: parsed.WorkloadID(),
		Name:       parsed.Name(),
		Build:      manifest.Build{Mode: parsed.BuildMode()},
	}
}

// resolve produces the manifest bytes, from flags alone when nothing may
// prompt and from the wizard otherwise. The directory it returns is where the
// project actually is: Detected.Dir, unless the interactive flow's directory
// screen chose another.
func (o Options) resolve(detected Detected) ([]byte, manifest.Draft, string, error) {
	if o.NonInteractive || !isStdinTerminalFn() {
		// Headless can warn but not offer: the check that interactively
		// becomes the directory question is a line on stderr here, and never
		// a refusal — a valid project cannot be reliably recognized, so a
		// wrong guess has to cost nothing. Only the headless paths return
		// detected.Dir unchanged, so the fourth value is settled right here.
		o.warnSuspectDir(detected)

		content, draft, err := o.resolveHeadless(detected)

		return content, draft, detected.Dir, err
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

	draft.EnvVars = o.storeSecrets(draft.EnvVars, detected, draft.Name)

	content, err := draft.Render()
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	return content, draft, nil
}

// warnSuspectDir says when the directory looks like the wrong place to run
// setup from — none of the usual project files — and names the subdirectories
// that look right, because the fix is one --dir away and this is the last
// moment before six questions and an artifact are spent on the wrong tree.
//
// An image-mode run is exempt: it never syncs local directory contents, so
// "everything here would be uploaded" describes a risk that cannot happen.
func (o Options) warnSuspectDir(detected Detected) {
	if o.Stderr == nil || !detected.SuspectDir() || o.Answers.BuildMode == manifest.BuildModeImage {
		return
	}

	w := o.Stderr

	line := fmt.Sprintf("Warning: %s has none of the usual project files (Dockerfile, pyproject.toml, "+
		"package.json, ...). If this is not the project root, everything here would be uploaded on deploy.",
		detected.Dir)

	if len(detected.Candidates) > 0 {
		names := make([]string, 0, len(detected.Candidates))
		for _, candidate := range detected.Candidates {
			names = append(names, candidate.Rel)
		}

		more := ""
		if detected.MoreCandidates {
			more = ", and more exist"
		}

		line += fmt.Sprintf(" These look like project roots: %s%s — pass --dir to use one.",
			strings.Join(names, ", "), more)
	}

	fmt.Fprintln(w, line)
}

// storeSecrets sends each secret to the credential store and reports what
// happened, so the render that follows writes credential ids rather than
// placeholders.
//
// It runs after the answers are settled and before anything is written, which
// is the only moment both facts are available: which variables the user calls
// secret, and what they are worth. Nothing is stored under --dry-run, because
// a run that promises to change nothing must not leave a credential behind.
func (o Options) storeSecrets(vars []manifest.EnvVar, detected Detected, workloadName string) []manifest.EnvVar {
	if o.DryRun {
		return vars
	}

	stored, report := importSecrets(vars, detected, workloadName)

	reportImport(o.Stderr, report)

	return stored
}

// resolveHeadlessBound binds to a live workload without prompting: the live
// spec is downloaded and the flags are applied on top, so a headless bind is
// the same operation the picker performs.
func (o Options) resolveHeadlessBound(detected Detected) ([]byte, manifest.Draft, error) {
	live, err := fetchLive(o.Answers.WorkloadID)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	if err := o.Answers.checkProbeEditable(live); err != nil {
		return nil, manifest.Draft{}, err
	}

	draft, err := o.Answers.applyTo(live.Defaults(), detected)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	// A name the workload already declares keeps its running value, so it is
	// not something this run added. Narrowing the draft here rather than
	// leaving it to Apply's own skip is what keeps the reported counts equal
	// to what reached the file, and keeps a credential from being stored for a
	// variable the workload is already serving.
	draft.EnvVars = live.NewEnvVars(draft.EnvVars)
	draft.EnvVars = o.storeSecrets(draft.EnvVars, detected, draft.Name)

	applied, err := live.Apply(draft)
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	content, err := applied.Render()
	if err != nil {
		return nil, manifest.Draft{}, err
	}

	// Read off the workload as it arrived, not off applied: the point is what
	// the platform was already serving in the clear, and a run that failed
	// above has no file to warn about.
	warnLiveSecretLiterals(o.Stderr, live)

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

// applyReadiness layers the port and the probe. Declining the probe is an
// answer like any other, and on a bound workload it removes the block the
// workload runs with, which the confirm screen shows as a diff before anything
// is written.
func (a Answers) applyReadiness(draft *manifest.Draft) {
	if a.Port != 0 {
		draft.Port = a.Port
	}

	if a.NoProbe {
		draft.HealthPath = ""

		return
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
	if a.Importance != "" {
		draft.Importance = a.importance()
	}

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

	return manifest.NewLive(workloadID, workloadDoc, artifactDoc)
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
		// Both sentinels mean the same thing here: there is no manifest above
		// to shadow this one. Not-a-directory says so because Locate refuses to
		// start the walk, and it names the ancestor it was handed rather than
		// the path the user typed, so a warning built from it reads as being
		// about a directory the reader has never seen. The project directory is
		// checked by the commands themselves, which name what the user typed.
		if errors.Is(err, manifest.ErrNotFound) || errors.Is(err, manifest.ErrNotADirectory) {
			return
		}

		fmt.Fprintf(stderr, "Warning: cannot check for a manifest above %s: %v\n", dir, err)

		return
	}

	fmt.Fprintf(stderr,
		"Warning: a manifest already exists at %s. Writing one in %s means a deploy from either directory reads a different file.\n",
		found, dir)
}

// warnEnvFile is warnUnreadEnvFile behind the opt-out: --skip-env means the
// user declined the import, so a failed read is not news either.
func (o Options) warnEnvFile(detected Detected) {
	if o.Answers.SkipEnv {
		return
	}

	warnUnreadEnvFile(o.Stderr, detected)
}

// checkEnvFile is warnEnvFile for a run that has somewhere to fail to. A .env
// the wizard could not read means the variables it holds reach nothing, and on
// a machine-readable run there is no stderr to say so on: the envelope's zero
// count reads exactly like a project that has no .env at all. So the same
// state that is a warning on a terminal is an error here.
//
// An import fails on it either way. Its whole job is to read that file, and
// "nothing to add" about a file it never parsed is the one answer it must not
// give.
func (o Options) checkEnvFile(detected Detected) error {
	if o.Answers.SkipEnv || detected.EnvErr == nil {
		o.warnEnvFile(detected)

		return nil
	}

	if o.JSONOutput || o.ImportEnv {
		return fmt.Errorf("cannot read %s: %w", EnvFileName, detected.EnvErr)
	}

	o.warnEnvFile(detected)

	return nil
}

// quiet reports that nothing should be printed: no writer, which is what the
// command hands over under --output-format json so stdout purity and
// `2>&1 | jq .` both hold without every reporter having to know why.
func (o Options) quiet() bool {
	return o.Stderr == nil
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

// warnLiveSecretLiterals names the variables a bound workload already declares
// in the clear whose values say what they are.
//
// The .env import classifies what it carries and writes a secret as a
// reference. The live spec gets no such treatment, by design: binding
// preserves what the workload is running, and rewriting a value the platform
// serves would change the running configuration on the next deploy. So a token
// someone pasted into the UI arrives verbatim in a file headed for git.
//
// Interactively the confirm screen is the answer, since the whole file is on
// screen before Enter writes it. Headlessly there is no screen, which is why
// this exists and why it is called from that path only. Names and the reason,
// never the value: this is printed to warn about disclosure and would
// otherwise be the disclosure.
//
// Uncapped, unlike the literal listing the command prints for a fresh
// manifest. That one lists everything ordinary and is bounded to stay
// readable; this lists only values carrying an issuer's own signature, of
// which any number is worth reading to the end.
func warnLiveSecretLiterals(stderr io.Writer, live manifest.Live) {
	if stderr == nil {
		return
	}

	named := make([]string, 0)

	for _, v := range live.LiteralEnvVars() {
		if reason, ok := evidentSecret(v.Value); ok {
			named = append(named, fmt.Sprintf("%s (%s)", v.Name, reason))
		}
	}

	if len(named) == 0 {
		return
	}

	fmt.Fprintf(stderr,
		"Warning: %s declares %d %s whose value looks like a secret, and binding copies it as it stands: %s.\n"+
			"  Replace the value in %s with a %s<credential-id>/<key> reference before committing the file.\n",
		live.Name, len(named), Plural(len(named), "variable", "variables"), strings.Join(named, ", "),
		manifest.FileName, manifest.CredentialShorthandPrefix)
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
