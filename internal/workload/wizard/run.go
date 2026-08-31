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
	// UpdateEnv brings the variables the manifest already declares back in
	// line with .env: a literal is rewritten, and the credential behind a
	// secret is re-sent. It is separate from ImportEnv because it is the
	// opposite act, changing what a name is worth rather than adding a name,
	// and because a secret cannot be checked first: the platform never hands a
	// stored value back, so this cannot tell a rotated key from an unchanged
	// one and re-sends either way.
	UpdateEnv bool
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
	// EnvValuesUpdated is how many declared literals this run rewrote to match
	// .env, and EnvSecretsRotated how many credentials it re-sent. They are
	// counted apart because they are different acts on different things: one
	// edits the committed file, the other writes to the tenant.
	EnvValuesUpdated  int
	EnvSecretsRotated int
	// EnvSecretsNotRotated is how many re-sends failed. A failed rotation
	// leaves the old value serving and the file unchanged, so without a count
	// of its own it is a silent failure for anything reading the result rather
	// than the terminal, which is exactly what --output-format json does.
	EnvSecretsNotRotated int
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

// editsEnv reports that this run acts on the .env of a manifest that already
// exists. Both flags read the file and both write something as a result, so
// every guard that turns on one of them turns on both.
func (o Options) editsEnv() bool {
	return o.ImportEnv || o.UpdateEnv
}

// checkNothingToImport refuses --import-env in a directory with no manifest.
// The flag names an existing file to add to, and falling through to setup
// would create one instead, in a directory the flag says was already
// configured, and mint credentials for the whole .env on the way.
func (o Options) checkNothingToImport(dir string) error {
	if !o.editsEnv() {
		return nil
	}

	// `up` reads the nearest manifest at or above its directory, so a project
	// laid out with the file at the repository root has one governing this
	// directory that this command, which only ever looks in the directory it
	// was given, would otherwise deny the existence of.
	if above, err := manifest.Locate(filepath.Dir(dir)); err == nil {
		return fmt.Errorf(
			"no %s in %s, so there is nothing to %s. The one at %s is what a deploy from here reads: "+
				"run this with --dir %s",
			manifest.FileName, dir, o.editVerb(), above, filepath.Dir(above))
	}

	return fmt.Errorf(
		"no %s in %s, so there is nothing to %s. "+
			"Run 'dr workload config' to create one, or point --dir at the project that has one",
		manifest.FileName, dir, o.editVerb())
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

	if o.editsEnv() {
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
	o.warnValueDrift(parsed, detected)

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

	// Asked once, up front, because it decides whether the comparison below
	// means anything at all. EnvVarNames answers the empty set for exactly the
	// shapes this refuses, and a container inheriting its environment through
	// an alias or a merge key really is serving names the walk cannot see:
	// compared against that empty set, an inherited block reads as a manifest
	// declaring nothing, and every name in .env reads as drift.
	blocked := parsed.CanDeclareEnvVars()

	// Through envVars rather than detected.EnvVars directly, so the notice
	// counts what --import-env would actually add: SkipEnv silences it, and a
	// variable the classifier called local-only is held back on purpose, so it
	// is reported separately below rather than as something the flag would fix.
	wanted := o.Answers.envVars(detected)

	// Naming a flag that cannot run on this file would be an errand, not
	// advice, and naming the variables it would add would be a guess. So the
	// count is of what .env offers rather than of a drift this cannot measure,
	// and the refusal says what the shape is and what to do instead.
	if blocked != nil {
		if len(wanted) > 0 {
			fmt.Fprintf(o.Stderr,
				"%s defines %d %s, and %s cannot be added automatically: %v\n",
				EnvFileName, len(wanted), Plural(len(wanted), "variable", "variables"),
				Plural(len(wanted), "it", "they"), blocked)
		}

		o.warnEnvHeldBack(parsed, detected, blocked)

		return
	}

	fresh := undeclared(wanted, parsed.EnvVarNames())
	if len(fresh) == 0 {
		o.warnEnvHeldBack(parsed, detected, blocked)

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
	o.warnEnvHeldBack(parsed, detected, blocked)
}

// warnValueDrift names the declared variables whose .env value no longer
// matches the manifest, and says what cannot be checked.
//
// Only the literals can be compared: a secret is a reference, and the platform
// never hands a stored value back, so a rotated key is indistinguishable from
// an untouched one. Saying that outright is the point. Silence about secrets
// would read as "those are fine", which is the reading that leaves a rotated
// key sitting in .env while the container serves the old one.
//
// Names only, never values. Both sides of a comparison are the user's own
// configuration and the classifier's verdict on which of them is sensitive is
// a heuristic.
func (o Options) warnValueDrift(parsed *manifest.Manifest, detected Detected) {
	if o.quiet() || o.Answers.SkipEnv {
		return
	}

	changed, unverifiable, reclassified, structured := o.compareValues(parsed, detected)

	// The values are read by the walk that resolves aliases and applied by the
	// one that refuses them, so a file can show its drift and still not accept
	// the edit. Where it would not, the refusal takes the place of the advice:
	// naming a flag that cannot run on this manifest is an errand, which is
	// the rule the missing-name notice already follows.
	blocker := parsed.CanDeclareEnvVars()

	if len(changed) > 0 {
		fmt.Fprintf(o.Stderr,
			"%s gives %d declared %s a different value from the manifest, and the manifest is what deploys: %s.\n%s",
			EnvFileName, len(changed), Plural(len(changed), "variable", "variables"),
			JoinNames(changed),
			applyLine(blocker, fmt.Sprintf("Apply %s with 'dr workload config --update-env'.",
				Plural(len(changed), "it", "them"))))
	}

	if len(reclassified) > 0 {
		fmt.Fprintf(o.Stderr,
			"%s now gives %d declared %s a value of a different kind from the one the manifest holds "+
				"(a literal where it stores a credential, or the reverse), which neither flag applies: %s.\n"+
				"  Change %s in %s by hand, or delete the %s and re-add %s.\n",
			EnvFileName, len(reclassified), Plural(len(reclassified), "variable", "variables"),
			JoinNames(reclassified), Plural(len(reclassified), "it", "them"), manifest.FileName,
			Plural(len(reclassified), "entry", "entries"), Plural(len(reclassified), "it", "them"))
	}

	if len(structured) > 0 {
		fmt.Fprintf(o.Stderr,
			"%s gives %d declared %s a value that is not a plain string, which neither flag rewrites: %s.\n"+
				"  Change %s by hand if %s is meant to win.\n",
			manifest.FileName, len(structured), Plural(len(structured), "variable", "variables"),
			JoinNames(structured), Plural(len(structured), "it", "them"), EnvFileName)
	}

	if len(unverifiable) > 0 {
		fmt.Fprintf(o.Stderr,
			"%d %s stored as %s, whose values cannot be compared with %s because the platform never returns them: %s.\n%s",
			len(unverifiable), Plural(len(unverifiable), "variable is", "variables are"),
			Plural(len(unverifiable), "a credential", "credentials"), EnvFileName,
			JoinNames(unverifiable),
			applyLine(blocker, "If one was rotated locally, re-send it with 'dr workload config --update-env'."))
	}
}

// applyLine is the second line of a drift notice: what to run about it, or why
// this manifest will not take it. The two flags refuse the same shapes before
// they touch anything, so a file that blocks one blocks the other.
func applyLine(blocker error, advice string) string {
	if blocker != nil {
		return fmt.Sprintf("  This manifest cannot be edited automatically: %v\n", blocker)
	}

	return "  " + advice + "\n"
}

// compareValues sorts the declared variables .env also defines into the ones
// whose value has moved and the ones nothing can answer for.
func (o Options) compareValues(
	parsed *manifest.Manifest, detected Detected,
) (changed, unverifiable, reclassified, structured []string) {
	local := make(map[string]manifest.EnvVar, len(detected.EnvVars))
	for _, v := range o.Answers.envVars(detected) {
		local[v.Name] = v
	}

	for _, declared := range parsed.DeclaredEnvVars() {
		want, defined := local[declared.Name]
		if !defined {
			continue
		}

		switch {
		// The classifier reads the value, so a value that changed enough can
		// change the verdict. Neither half acts on that: an update matches
		// like for like and an import skips a declared name. Naming it is all
		// that stops it being the one drift nothing ever mentions.
		case declared.Secret() != want.Secret:
			reclassified = append(reclassified, declared.Name)
		// A placeholder has no stored value to compare or re-send.
		// warnEnvHeldBack already names it, and saying "rotate it with
		// --update-env" would be advice for a command that skips it.
		case declared.CredentialID == manifest.CredentialPlaceholder:
			continue
		case declared.Secret():
			unverifiable = append(unverifiable, declared.Name)
		// A value written as a mapping or a list. The update path will not
		// overwrite one, so calling it drift would advise a flag that then
		// does nothing.
		case declared.Structured:
			structured = append(structured, declared.Name)
		case declared.Value != want.Value:
			changed = append(changed, declared.Name)
		}
	}

	return changed, unverifiable, reclassified, structured
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
//
// blocked is why the manifest's declared names cannot be read, or nil when
// they can. The local-only line rests on that set and is dropped without it:
// a container inheriting its environment may already carry every one of these
// names, and calling them deliberately omitted would send the user off to add
// by hand a variable the deploy already has. The placeholder line stands
// either way, because it resolves aliases and so reads the same entries the
// deploy does.
func (o Options) warnEnvHeldBack(parsed *manifest.Manifest, detected Detected, blocked error) {
	if o.quiet() || o.Answers.SkipEnv {
		return
	}

	if blocked == nil {
		o.warnEnvLocalOnly(parsed, detected)
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

// warnEnvLocalOnly names the .env variables the classifier held back and the
// manifest does not carry either, because a verdict the user never sees is one
// they have no way to overrule.
//
// Only sound where the manifest's declared names can be read: see warnEnvHeldBack.
func (o Options) warnEnvLocalOnly(parsed *manifest.Manifest, detected Detected) {
	var local []string

	declared := parsed.EnvVarNames()

	for _, v := range detected.EnvVars {
		if v.Kind == EnvLocal && !declared[v.Name] {
			local = append(local, v.Name)
		}
	}

	if len(local) == 0 {
		return
	}

	fmt.Fprintf(o.Stderr,
		"%s defines %s that %s read as local-only, so %s deliberately left out of the manifest: %s.\n"+
			"  Add %s by hand if the workload needs %s.\n",
		EnvFileName, Plural(len(local), "a variable", "variables"),
		Plural(len(local), "was", "were"), Plural(len(local), "it is", "they are"),
		JoinNames(local),
		Plural(len(local), "it", "them"), Plural(len(local), "it", "them"))
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
	// Only the additive half needs somewhere to append. A rotation writes to
	// the credential store and a literal rewrite edits an entry already there,
	// so refusing those for "nowhere to add a variable" would be an
	// add-shaped refusal of a run that adds nothing.
	if o.ImportEnv {
		if err := parsed.CanDeclareEnvVars(); err != nil {
			return Result{}, err
		}
	}

	wanted := o.Answers.envVars(detected)

	result := Result{Path: path, Action: ActionUnchanged, Draft: draftOf(parsed)}

	if o.UpdateEnv {
		if err := o.updateEnv(path, parsed, detected, wanted, &result); err != nil {
			return Result{}, err
		}
	}

	if o.ImportEnv {
		if err := o.addEnv(path, parsed, detected, wanted, &result); err != nil {
			return Result{}, err
		}
	}

	// A rotation leaves the file untouched, so the action alone cannot tell a
	// run that did nothing from one that re-sent every secret it found, and
	// only the first of those has "nothing to do" to report.
	if result.Action == ActionUnchanged && result.EnvSecretsRotated == 0 && result.EnvSecretsNotRotated == 0 {
		o.reportNothingToImport(parsed, detected)

		return result, nil
	}

	// What the flags would not touch is news whether or not they touched
	// something: a name held back is held back either way, and a placeholder
	// left by an earlier run is still what the next deploy refuses.
	//
	// nil because this path is downstream of the CanDeclareEnvVars above, so
	// the declared names the local-only line rests on are readable.
	o.warnEnvHeldBack(parsed, detected, nil)

	return result, nil
}

// updateEnv brings declared variables back in line with .env: a literal is
// rewritten in the file, and the credential behind a secret is re-sent.
//
// The secrets are re-sent without being checked, because they cannot be: the
// platform never returns a stored value. That is why this needs asking for.
// Re-sending a value that was already stored changes nothing, so the cost of
// the unnecessary case is one request, and the cost of not doing it is a
// container serving a key its owner has already rotated.
func (o Options) updateEnv(
	path string, parsed *manifest.Manifest, detected Detected, wanted []manifest.EnvVar, result *Result,
) error {
	// The rewrite is proven before a single credential is re-sent. A PATCH is
	// irreversible and the file edit can still refuse for reasons the parsed
	// tree cannot see, so running it as a preview first is what stops a
	// refused run from having already overwritten the tenant's secrets.
	if _, _, err := manifest.UpdateEnvVars(path, wanted, true); err != nil {
		return err
	}

	rotated, failed := o.rotateSecrets(parsed, detected, wanted)

	changed, content, err := manifest.UpdateEnvVars(path, wanted, o.DryRun)
	if err != nil {
		return err
	}

	result.EnvValuesUpdated = len(changed)
	result.EnvSecretsRotated = rotated
	result.EnvSecretsNotRotated = failed

	if len(changed) == 0 && rotated == 0 && failed == 0 {
		return nil
	}

	// A rotation is a write to the credential store, not to the file, so it
	// does not make the run an edit: saying "updated .datarobot.yaml" about a
	// file that is byte for byte what it was is the kind of claim this command
	// exists not to make. A dry run is planned either way, because both are
	// things it would have done.
	if len(changed) > 0 || o.DryRun {
		result.Action = o.editAction()
	}

	if len(changed) == 0 {
		return nil
	}

	if len(content) > 0 {
		result.Content = content
	}

	result.Draft.EnvVars = append(result.Draft.EnvVars, changed...)

	return nil
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

// rotateSecrets re-sends each declared secret's current .env value to the
// credential the manifest points at, and reports how many it sent and how many
// it could not.
//
// Nothing is created: the id in the file stays the id in the file, so a
// rotation leaves the manifest untouched. That is also why it does not reach a
// workload on its own: a container reads its credentials at startup, and `up`
// compares the manifest it already deployed, finds it unchanged and does
// nothing. Only a restart makes the new value the one being served, which is
// what the command says after a run that re-sent something. A send that fails is reported and
// does not stop the others, matching how the import treats a store it cannot
// reach: a partial result is the ordinary failure here, not an exceptional
// one. The count of those comes back because a failure leaves the old value
// serving and the file with nothing to show for it, so it is the only trace a
// caller reading the result rather than the terminal would ever get.
func (o Options) rotateSecrets(
	parsed *manifest.Manifest, detected Detected, wanted []manifest.EnvVar,
) (rotated, failedCount int) {
	// Only the names the answers kept. .env is read through the classifier for
	// a reason: a value it reads as local-only is one the workload must not
	// get, and --skip-env says not to read the file at all. Indexing the raw
	// detection instead would send a developer's localhost URL to the tenant
	// credential a production workload reads.
	send := make(map[string]bool, len(wanted))

	for _, v := range wanted {
		if v.Secret {
			send[v.Name] = true
		}
	}

	values := secretValues(detected)

	var failed []ImportFailure

	for _, declared := range parsed.DeclaredEnvVars() {
		value, reason := rotatable(declared, send, values, parsed.Name())
		if reason != nil {
			failed = append(failed, *reason)

			continue
		}

		if value == "" {
			continue
		}

		// A dry run counts what it would send and sends nothing. Saying
		// nothing about the secrets instead would leave the one part of
		// --update-env that reaches the tenant out of the preview of it.
		if o.DryRun {
			rotated++

			continue
		}

		if _, err := updateCredentialFn(declared.CredentialID, value); err != nil {
			failed = append(failed, ImportFailure{Name: declared.Name, Reason: importReason(err, declared.Name)})

			continue
		}

		rotated++
	}

	reportRotation(o.Stderr, rotated, failed, o.DryRun)

	return rotated, len(failed)
}

// rotatable is the value to re-send for a declared variable, empty when this
// one is not a rotation candidate, and a failure when it is one this command
// must not perform.
//
// The ownership check is the important half. A credential id in the manifest
// may have been pasted by hand, and the store is tenant-wide: overwriting one
// that other workloads read, from a flag about this project's .env, is the
// damage this refuses. Only a credential named the way setup names them, and
// only its apiToken field, is one this command wrote and may write again.
func rotatable(
	declared manifest.DeclaredEnvVar, send map[string]bool, values map[string]string, workloadName string,
) (string, *ImportFailure) {
	if !declared.Secret() || !send[declared.Name] ||
		declared.CredentialID == manifest.CredentialPlaceholder {
		return "", nil
	}

	// Only a credential this CLI would have written. The reference names which
	// field of the credential feeds the variable, and re-sending an apiToken to
	// one holding a password would rewrite a credential other DataRobot assets
	// share, from a flag that promised to touch this workload's environment.
	if declared.CredentialKey != manifest.CredentialKeyWritten {
		return "", &ImportFailure{
			Name: declared.Name,
			Reason: fmt.Sprintf("it reads the %q field of its credential, which this command does not write; "+
				"rotate that credential in DataRobot instead", declared.CredentialKey),
		}
	}

	// A read that fails is not proof of anything, so it refuses too: the cost
	// of not re-sending is a stale key the user is told about, and the cost of
	// re-sending blind is another workload's secret replaced.
	want := CredentialName(workloadName, declared.Name)

	cred, err := getCredentialFn(declared.CredentialID)
	if err != nil {
		return "", &ImportFailure{Name: declared.Name, Reason: importReason(err, want)}
	}

	if cred.Name != want {
		return "", &ImportFailure{
			Name: declared.Name,
			Reason: fmt.Sprintf("it points at credential %q, which this project did not create and other "+
				"workloads may share; rotate that credential in DataRobot instead", cred.Name),
		}
	}

	return values[declared.Name], nil
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
		fmt.Fprintf(o.Stderr, "No %s in %s, so there is nothing to %s.\n",
			EnvFileName, dir, o.editVerb())

		return
	}

	// A file the flags cannot act on is not a file that agrees with .env, so
	// where there is drift left over the notice about it takes the place of
	// the summary: a run that says both would contradict itself in two lines.
	if o.hasUnappliedDrift(parsed, detected) {
		o.warnValueDrift(parsed, detected)
	} else {
		fmt.Fprintf(o.Stderr, "%s %s.\n", ShortPath(parsed.Path), o.nothingLeftToDo())
	}

	// An update alone says nothing about names the manifest is missing, so the
	// drift notice still owes them: without it the run reads as a clean bill
	// of health for a file that is short a variable.
	if !o.ImportEnv {
		o.warnEnvDrift(parsed, detected)
	}

	// nil because editEnv refuses a manifest whose declared names cannot be
	// read before it ever gets as far as importing nothing.
	o.warnEnvHeldBack(parsed, detected, nil)
}

// hasUnappliedDrift reports whether an update that changed nothing left a
// value behind that it was never going to settle: one the file holds as a
// mapping or a list, or one whose kind no longer matches what the manifest
// stores. Both are things --update-env skips on purpose, and both make
// "already gives every variable the value .env does" a false summary.
//
// Only for a run that asked to update. A literal .env changed is the other
// flag's business, and an import saying nothing to add is still true about it.
func (o Options) hasUnappliedDrift(parsed *manifest.Manifest, detected Detected) bool {
	if !o.UpdateEnv {
		return false
	}

	_, _, reclassified, structured := o.compareValues(parsed, detected)

	return len(reclassified)+len(structured) > 0
}

// editVerb names the act that found nothing to do, so a run that only asked
// for one of the two flags is not told about the other.
func (o Options) editVerb() string {
	if !o.ImportEnv {
		return "update"
	}

	return "import"
}

// nothingLeftToDo is what a file already in the state the flags would put it
// in has to say for itself, which is a different sentence per flag: one is
// about the names it carries, the other about the values behind them.
func (o Options) nothingLeftToDo() string {
	declares := fmt.Sprintf("already declares every variable in %s that can be imported", EnvFileName)
	matches := fmt.Sprintf("already gives every variable it declares the value %s does", EnvFileName)

	switch {
	case o.ImportEnv && o.UpdateEnv:
		return declares + ", and " + matches
	case o.UpdateEnv:
		return matches
	default:
		return declares
	}
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

	if o.JSONOutput || o.editsEnv() {
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
