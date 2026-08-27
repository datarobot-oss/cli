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

package up

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/datarobot/cli/tui"
)

// Test seams. The deploy is mostly network, and the tests are not.
var (
	runWizardFn        = wizard.Run
	createWorkloadFn   = workload.CreateWorkload
	waitWorkloadFn     = workload.WaitForWorkload
	waitSteadyFn       = workload.WaitForSteadyWorkload
	listWorkloadsFn    = workload.ListWorkloads
	startWorkloadFn    = workload.StartWorkload
	createArtifactFn   = workload.CreateArtifact
	getArtifactFn      = workload.GetArtifact
	lockArtifactFn     = workload.LockArtifact
	triggerBuildFn     = workload.TriggerArtifactBuild
	waitBuildFn        = workload.WaitForBuild
	listBuildsFn       = workload.ListArtifactBuilds
	getCredentialFn    = workload.GetCredential
	findCredentialFn   = workload.FindCredentialNamed
	guardReplacementFn = workload.RefuseActiveReplacement
	startReplacementFn = workload.StartReplacement
	waitReplacementFn  = workload.WaitForReplacement
	updateSettingsFn   = workload.UpdateWorkloadSettings
	writeWorkloadIDFn  = manifest.WriteWorkloadID
	codeChangeFn       = defaultCodeChange
	projectLinkedFn    = wapi.Exists
	loadProjectFn      = wapi.LoadConfig
	initProjectFn      = wapi.Initialize
	saveProjectFn      = wapi.SaveConfig
	patchCodeRefFn     = workload.PatchArtifactCodeRef
	syncProjectFn      = defaultSync
)

// Options is everything a run needs from its caller.
type Options struct {
	// Dir is where to start looking for the manifest. The search walks
	// upward from here.
	Dir string

	// NonInteractive forbids prompting. With no manifest it turns the setup
	// wizard into an error naming the command that writes one.
	NonInteractive bool

	// DryRun stops after the plan.
	DryRun bool

	// Detach returns once the apply is requested, skipping the waits.
	Detach bool

	// ForceBuild rebuilds the image even when the working tree matches what
	// was last synced. The deploy skips a build it believes would produce the
	// image already on the artifact; this is the way to say it is wrong,
	// which it can be when code reached the artifact through
	// `dr artifact code sync` rather than through a deploy.
	ForceBuild bool

	// Confirm asks the user a question that only typing want exactly may
	// answer. It is used once, before rolling a locked production version,
	// and nil when there is no terminal to ask on. Reading the answer is the
	// caller's job because only it knows where the user's input comes from.
	Confirm func(question, want string) (bool, error)

	// Lock makes the artifact that ends up live immutable and permanent.
	// Locking is one-way, so it happens last, only after the workload is
	// actually serving: locking something that never came up would leave an
	// undeletable artifact behind.
	//
	// It is about the end state, not about what this run happened to do. A
	// deploy that mints no version still locks the one that is serving, which
	// is what a start and a sizing change both do. The alternative would be a
	// flag that sometimes locks and sometimes does not depending on which
	// fields the file moved, and the run would have to be read to know which.
	Lock bool

	// PollInterval and PollTimeout tune the waits.
	PollInterval time.Duration
	PollTimeout  time.Duration

	// Stderr takes the plan and the progress. Never stdout: that belongs to
	// the endpoint, or to one JSON document.
	Stderr io.Writer

	// Spinner is true only when Stderr is the terminal the user is watching.
	Spinner bool
}

// Result is what happened, and the material for the JSON envelope.
type Result struct {
	Plan Plan

	WorkloadID string
	Name       string
	Status     string
	Endpoint   string
	ArtifactID string
	Action     string
	Locked     bool

	// BuildID names the image build this run ran, empty when it ran none:
	// either the manifest points at a published image, or the one already on
	// the artifact was still current. A build that failed still sets it, so
	// the caller can say which logs to read.
	BuildID string
}

// Run reads, plans, and applies as much of the plan as this release can.
func Run(opts Options) (Result, error) {
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("cannot resolve %s: %w", opts.Dir, err)
	}

	loaded, err := load(dir, opts)
	if err != nil {
		return Result{}, err
	}

	live, err := lookSettled(loaded.WorkloadID(), opts)
	if err != nil {
		// The binding travels with the failure. A run that dies before the plan
		// exists still has to report which workload it was about: losing the id
		// is how a deploy becomes unfindable, and it is the whole of what the
		// JSON envelope can carry from here.
		return Result{WorkloadID: loaded.WorkloadID()}, err
	}

	code, err := codeChangeFn(loaded, live)
	if err != nil {
		return Result{}, err
	}

	noteIgnoreFile(code, opts)

	plan, err := Build(loaded, live, code)
	if err != nil {
		return Result{}, err
	}

	// Read here rather than in Build, which is kept off the filesystem so its
	// tests can stay there too.
	plan.LinkedArtifact = projectLinkedFn(loaded.ProjectDir)

	result := Result{
		Plan:       plan,
		WorkloadID: boundID(live),
		Name:       name(loaded, live),
		Status:     plan.State.String(),
		Endpoint:   live.Endpoint,
		ArtifactID: live.ArtifactID,
		// Action records what this run did, not what the plan wanted, so it
		// starts at "nothing" and each path below sets its own once it has
		// acted. Seeding it from the plan reported a mutation for every run
		// that failed before attempting one, including the two returns just
		// below this. What was wanted stays readable under plan.action.
		Action: ActionUnchanged,
		Locked: live.Locked,
	}

	if err := bindLocked(loaded, plan, &result); err != nil {
		return result, err
	}

	summary := Summary{Name: result.Name, WorkloadID: result.WorkloadID, Status: live.Status}
	if err := Render(opts.Stderr, summary, plan); err != nil {
		return result, err
	}

	noteUnusedForce(plan, opts)

	if opts.DryRun {
		// The one run whose action is the plan's: it stops here, so printing
		// the plan is the whole of what it did.
		result.Action = plan.Action()

		return result, nil
	}

	if plan.Empty() {
		return lockOnly(loaded, live, result, opts)
	}

	return apply(loaded, live, plan, result, opts)
}

// lockOnly is the whole of a --lock run that found nothing else to do.
//
// --lock is about the end state rather than about what this run happened to
// change: the flag says the artifact that ends up live should be permanent,
// and an empty plan means the one already serving is that artifact. Returning
// early on Empty, as this used to, printed "Already up to date" and exited 0
// having locked nothing, which is the wrong answer twice over. It contradicts
// the flag's own help, and it breaks the sequence the draft warning tells
// people to follow: deploy, read the warning, run 'up --lock'. By then there
// is nothing left to change, so the remedy silently did nothing at all.
//
// The live state is checked first because an empty plan is not the same as a
// healthy workload. Only a stopped one is excluded by Empty itself; an errored
// workload with no drift still lands here, and locking cannot be undone, so
// this refuses rather than making something permanent out of something broken.
func lockOnly(loaded Loaded, live Live, result Result, opts Options) (Result, error) {
	if !opts.Lock || result.Locked {
		return result, nil
	}

	// loaded is threaded through for the remedy a refusal names: a terminated
	// workload is cleared by `dr workload delete`, and that command only finds
	// this project's manifest if the message carries the same --dir the deploy
	// was given.
	if err := deployable(live, result.Name, dirFlagFor(loaded)); err != nil {
		return result, err
	}

	// A rollout in flight is about to move the workload off the artifact this
	// would make permanent, and locking cannot be undone. deployable does not
	// cover it: the workload reports itself running for the whole of a swap,
	// so the only way to know is to ask.
	if err := guardRollout(live.WorkloadID, "the artifact was left as it is"); err != nil {
		return result, err
	}

	return lock(result, newReporter(opts.Stderr, opts.Spinner))
}

// guardRollout refuses to act while a swap is already under way, and gives a
// failure to find out the operation it stopped.
//
// It asks the narrow question rather than the guarded one. The full guard
// spends a second GET disambiguating the replacement route's 404 between
// "nothing in flight" and "no such workload"; every caller here reaches this
// through Look, which already read the workload, and deployable, which already
// refused the states where it is gone. So the ambiguity is settled before the
// question is asked, and paying for it again would put a redundant round trip
// on the quiet path of every deploy.
//
// Only the second kind of error is wrapped. The refusal already names the
// workload, what is in flight and the remedy, so prefixing it would put
// "cannot tell whether" in front of a sentence that plainly knows. A
// replacement route that answered 500, or timed out, says nothing at all
// about the run it just stopped, and the reader is looking at a deploy rather
// than at a route.
func guardRollout(workloadID, consequence string) error {
	err := guardReplacementFn(workloadID)
	if err == nil || errors.Is(err, workload.ErrReplacementInFlight) {
		return err
	}

	return fmt.Errorf("cannot tell whether workload %s already has a rollout in progress, so %s: %w",
		workloadID, consequence, err)
}

// lookSettled is the live read a plan is built from: the workload as it is,
// once it has stopped moving.
func lookSettled(workloadID string, opts Options) (Live, error) {
	found, err := Look(workloadID)
	if err != nil {
		return Live{}, err
	}

	return awaitSteady(found, opts)
}

// awaitSteady waits out a workload that is still moving, and hands back what
// it settled into.
//
// It runs before the plan rather than as a refusal after it, which is the whole
// change: a deploy arriving mid-transition is early rather than wrong, and
// telling someone to run the same command again in a minute is work the command
// can do itself. The state it waits for is a destination, not a healthy one, so
// a workload that settles into errored or stopped goes on to the paths that know
// what to do about those.
//
// It is the workload's own status that is waited on, which is narrower than it
// sounds: a workload being replaced reports itself running for the whole of the
// swap, so a rollout in flight is not something this can see. That case is
// caught by guardRollout instead, off the replacement route.
//
// The workload is re-read afterwards rather than patched with the status the
// wait ended on. The status is only half of what the plan is built from, and
// the other half is the spec: reading one from before the transition and one
// from after would compare a file against a snapshot that never existed.
//
// A dry run never waits, because blocking a preview for the poll timeout is the
// opposite of what a preview is for. What it must not do is pretend it knows
// the answer: the plan for a workload halfway through a transition depends on
// where the transition lands, so the state travels on unchanged and Render
// declines to call it up to date.
func awaitSteady(live Live, opts Options) (Live, error) {
	if live.State != StateSettling {
		return live, nil
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	if opts.DryRun {
		report.say("  %s\n", tui.HintStyle.Render(
			"A deploy would wait for this to finish and plan against where it lands."))

		return live, nil
	}

	// Said before the wait, not after it. Without a terminal there is no
	// spinner and a phase prints nothing until it ends, so a CI log would
	// otherwise show no output at all for as long as the transition takes,
	// and the plan block that names the workload comes later still.
	report.say("  %s\n", tui.HintStyle.Render(fmt.Sprintf(
		"Workload %s is %s.", live.WorkloadID, live.Status)))

	if opts.Detach {
		// --detach is about not waiting for the deploy to serve. This wait is
		// before the deploy: what to apply cannot be known until the workload
		// stops moving. Saying so beats blocking in silence.
		report.say("  %s\n", tui.HintStyle.Render(
			"Waiting for it to settle before planning; --detach applies to the deploy."))
	}

	var settled *workload.Workload

	err := report.run("Waiting for the workload to settle", func() error {
		wl, waitErr := waitSteadyFn(live.WorkloadID, opts.PollInterval, opts.PollTimeout, nil)
		settled = wl

		return waitErr
	})
	if err != nil {
		if failure := settleFailed(live, settled, err); failure != nil {
			return live, failure
		}

		// The workload went away while this was waiting. That is drift, and
		// the read below is what says so.
	}

	return Look(live.WorkloadID)
}

// settleFailed explains a wait that did not land, and tells a workload that
// went away from one that is simply slow.
//
// A 404 is not a failure to settle. Look treats a workload the platform does
// not have as drift and lets the deploy recreate it, and a workload deleted
// while this was waiting deserves the same answer rather than a message
// pointing at an id that no longer exists. Returning nil hands the run back to
// the re-Look above, which reports StateMissing.
//
// Everything else names where the transition got to, which the wait returns
// precisely so a caller can say it: "still stopping after 30m" is what decides
// whether to wait longer or go and look at the platform, and the bare timeout
// says neither.
func settleFailed(live Live, settled *workload.Workload, err error) error {
	if isNotFound(err) {
		return nil
	}

	where := "did not finish settling"
	if settled != nil && settled.Status != "" {
		where = "was still " + settled.Status
	}

	return fmt.Errorf(
		"workload %s %s, so nothing was deployed; check 'dr workload status %s': %w",
		live.WorkloadID, where, live.WorkloadID, err)
}

// noteIgnoreFile passes on the note about a deprecated ignore filename.
// Sizing the working tree is the whole of what a --dry-run does, so this is
// the only point on that path where there is anything to say.
//
// Only the housekeeping note comes through here. The ignore-file problems that
// cost the user something are logged by the phase that finds them, so they
// survive a run that fails before reaching this line.
func noteIgnoreFile(code CodeChange, opts Options) {
	if code.IgnoreNotice == "" {
		return
	}

	fmt.Fprintf(opts.Stderr, "  %s\n", tui.HintStyle.Render(code.IgnoreNotice))
}

// bindLocked settles Locked for the one shape live state cannot answer: a
// manifest bound by artifact id, deploying to a workload that does not exist
// yet. Locked is otherwise read off the artifact the workload is running, and
// a create has no workload to read it from, so the result would say draft
// about an artifact that may well be permanent.
//
// That is not a cosmetic slip. Deploying a locked, versioned artifact onto a
// fresh workload is how a promotion works, and getting it wrong there tells
// someone their permanent deploy is temporary and then advises 'up --lock',
// which the platform answers with a 403 because the artifact is already
// locked. The reader is left with a warning they cannot act on.
//
// Only creates ask. A roll onto a live workload cannot hit this, because the
// platform refuses to swap a draft for a locked version and the run fails on
// its own terms long before anything is printed.
//
// A failure to read is returned rather than shrugged off. The id came from
// the file, so an artifact that cannot be read is one the deploy was going to
// fail on anyway, and saying so here costs a create instead of a plan that
// was never true.
func bindLocked(loaded Loaded, plan Plan, result *Result) error {
	if !plan.Creates || result.Locked {
		return nil
	}

	bound := loaded.Compiled.ArtifactID
	if bound == "" {
		return nil
	}

	locked, err := lockedAlready(bound)
	if err != nil {
		return err
	}

	result.Locked = locked

	return nil
}

// noteUnusedForce reports a --force-build that is about to do nothing,
// because a run which stops without saying so reads as a rebuild that quietly
// happened. A dry run is excluded: it changes nothing by definition, and the
// flag not having been used is the least of what it did not do.
//
// There are two ways for the flag to be idle. A run that deploys nothing is
// the loud one. The quiet one is a manifest naming a published image: the
// deploy goes ahead and succeeds, so nothing invites a second look at the
// image it is actually serving, and the flag asked for a rebuild of something
// this project never builds.
func noteUnusedForce(plan Plan, opts Options) {
	if !opts.ForceBuild || opts.DryRun {
		return
	}

	switch {
	case plan.Empty():
		fmt.Fprintf(opts.Stderr, "  --force-build had no effect: nothing is being deployed.\n")

	case !plan.Code.Applies:
		fmt.Fprintf(opts.Stderr,
			"  --force-build had no effect: this manifest names an image the platform does not build.\n")
	}
}

// load finds the manifest, running the setup wizard when there is none and a
// person is there to answer it. In CI there is nobody, so a missing manifest
// is an error that names the command which writes one: deploying by guessing
// is the one thing this command must never do.
func load(dir string, opts Options) (Loaded, error) {
	loaded, err := Load(dir)
	if err == nil || !errors.Is(err, ErrNoManifest) {
		return loaded, err
	}

	if opts.NonInteractive {
		return Loaded{}, fmt.Errorf(
			"%w. Run 'dr workload config' to create one, then deploy", err)
	}

	if _, err := runWizardFn(wizard.Options{Dir: dir, Stderr: opts.Stderr}); err != nil {
		return Loaded{}, err
	}

	return Load(dir)
}

// boundID is the workload this run is about, for the envelope and for a
// failure that has to report something.
//
// A binding that resolved to nothing is deliberately not it. Nothing answers
// to that id, so handing it to a caller gives them something whose only use is
// a 404, and a run that then fails to create would have reported a workload
// that has never existed. The create overwrites this on success; the plan
// carries the dead id separately, for a reader rather than for a script.
//
// Keyed on the state rather than on plan.PriorWorkloadID, which today tracks
// the same thing. Deciding what the envelope reports by testing a render field
// welds the two together across files: the moment anything populates that
// field for a second state, this empties WorkloadID, reportable() goes false,
// and the whole JSON document disappears from stdout on a failed run.
func boundID(live Live) string {
	if live.State == StateMissing {
		return ""
	}

	return live.WorkloadID
}

// dirFlagFor is the " --dir <path>" a remedy has to carry to reach this
// project, and empty when the working directory already does.
//
// A message that names `dr workload delete <id>` without it is false in
// exactly the layout --dir was added for: a project one level down is
// invisible to a search that only walks upward, so the command runs, finds
// nothing, and prints nothing. `delete` prints the same suffix back at `up`,
// so both go through one implementation rather than two notions of "already
// there" that can disagree about one directory.
func dirFlagFor(loaded Loaded) string {
	return manifest.DirFlag(loaded.ProjectDir)
}

// name is what to call this workload: the live one's name once it exists, the
// file's before that.
func name(loaded Loaded, live Live) string {
	if live.Name != "" {
		return live.Name
	}

	return loaded.Manifest.Name()
}

// apply carries out the plan, or explains why it cannot.
func apply(loaded Loaded, live Live, plan Plan, result Result, opts Options) (Result, error) {
	if err := deployable(live, result.Name, dirFlagFor(loaded)); err != nil {
		return result, err
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	// Everything that can refuse this run goes first, because a start is a
	// mutation and the branch below performs one. The order used to be
	// academic: a stopped workload with drift was turned away here, so nothing
	// could refuse after something had already changed.
	lock, err := beforeAnything(loaded, live, plan, result, opts)
	if err != nil {
		return result, err
	}

	// A stopped workload is started before anything else. Everything below
	// changes a workload that has to be running to receive it: a rollout
	// promotes onto something serving, and a settings change is followed by a
	// wait for the workload to come back.
	//
	// This used to refuse whenever the file asked for more than a start, on the
	// grounds that coming up on the version it was stopped on and then failing
	// was the worst of both. It is not: the user asked for the workload to be
	// up, so a failed roll that leaves it up on the old version is exactly what
	// a failed roll against a running workload leaves behind, while the refusal
	// left it switched off and told the reader to run two more commands.
	//
	// Starting first is also what keeps this one deploy rather than two. From
	// the line below, a run against a stopped workload is indistinguishable
	// from a run against a running one. live is deliberately not rewritten to
	// say so: nothing below reads its state or status, and the fields that are
	// read describe the artifact rather than the workload, so faking a refresh
	// would advertise a consistency this value does not have.
	if live.State == StateStopped {
		result, err = startFirst(live, plan, result, opts, report)
		if err != nil || plan.OnlyStarts() {
			return result, err
		}
	}

	// A runtime-only change is applied to the workload in place, with no new
	// version minted. Asked through the plan rather than restated here, so the
	// branch and the credential skip that mirrors it cannot drift apart: they
	// are the same question, and answering it twice is how a roll comes to skip
	// a check or a resize comes to fail one.
	if plan.Retunes() {
		return retune(loaded, result, opts, report)
	}

	// A workload that already exists is replaced rather than created: the
	// endpoint has to survive, and something is serving on it meanwhile.
	if plan.RollsArtifact() {
		return roll(loaded, live, plan, lock, result, opts, report)
	}

	// A published image is one POST. Anything the platform builds has to be
	// given somewhere to put the code and time to turn it into an image
	// first, which is a different shape of deploy rather than a longer one.
	if plan.Code.Applies {
		return buildAndCreate(loaded, plan.Code, result, opts, report)
	}

	return create(loaded, loaded.Compiled.Payload, result, opts, report)
}

// beforeAnything runs every check that can still refuse, and settles the one
// question that needs a person, so that nothing below can turn the run away
// after it has already changed something. It reports whether the version this
// run rolls has to be locked.
//
// The ordering matters now in a way it did not before. A stopped workload with
// drift used to be refused outright, so the credential check, the in-flight
// rollout guard and the locked-production confirm all sat comfortably after it.
// Now the run starts the workload first, and a refusal that came later would
// leave production switched on and serving traffic it was deliberately stopped
// from serving, under a message saying nothing had changed.
func beforeAnything(loaded Loaded, live Live, plan Plan, result Result, opts Options) (bool, error) {
	// The only check here that needs the network for the file's own sake: a
	// credential id is the one thing the local ledger cannot judge. It runs
	// once rather than per branch: a stopped workload with drift now reaches
	// both the start and the roll, each of which used to verify for itself,
	// and the check is a round trip per reference.
	if err := verifyCredentials(credentialRefs(loaded, plan), result.Name); err != nil {
		return false, err
	}

	if !plan.RollsArtifact() && len(plan.Runtime) == 0 {
		return false, nil
	}

	// A swap already in flight is about to move the workload somewhere this run
	// has not planned for. roll and retune each guard immediately before their
	// own call, which is the check that actually holds and which covers a
	// workload that is already up; this one exists for the workload that is
	// not, so a doomed run does not start something it is then going to refuse
	// to deploy onto.
	if live.State == StateStopped {
		if err := guardRollout(live.WorkloadID, "the workload was left stopped"); err != nil {
			return false, err
		}
	}

	if !plan.RollsArtifact() {
		return false, nil
	}

	// Asked before the first mutation rather than after the candidate is
	// minted. Agreeing still costs nothing that cannot be undone, since the
	// lock itself is taken immediately before the swap; declining now costs
	// neither a draft nor a workload switched on to be rolled off again.
	return confirmLock(live, result.Name, opts)
}

// credentialRefs is what this run has to verify before it mutates anything,
// and nothing at all for a change that only moves the sizing of something
// already running.
//
// That exception is the deliberate one: a runtime-only change sends no
// environment at all, so the artifact keeps every variable it already has and a
// credential this deploy does not touch must not be able to refuse a resize.
//
// A stopped workload is not covered by it, even when sizing is all its file
// moved, because the run starts the workload as well. A start is when the
// container resolves its references, so a credential deleted while the workload
// was off would otherwise surface minutes later as a container that will not
// come up. Every remaining run either builds a version out of the references or
// starts a container that reads them, so it verifies.
func credentialRefs(loaded Loaded, plan Plan) []manifest.CredentialRef {
	if plan.SendsNoEnvironment() {
		return nil
	}

	return loaded.Compiled.CredentialRefs
}

// deployable refuses the live states that cannot take a deploy, one message
// each. None is a guess: an unsettled workload may still be on its way
// somewhere, an errored one has nothing safe underneath it, and a terminated
// one is not coming back.
//
// Missing is not among them. A binding that resolves to nothing is drift, and
// the apply for it is a create; the plan says which id went missing so a
// wrong-instance run is visible rather than silent.
//
// Terminated is, even though it too names something that will never run again.
// The difference is that it still exists: it holds its name and its artifact,
// so a create collides with itself, and the message has to name the one thing
// that clears the way rather than pretend the workload is gone.
//
// Stopped is not among them either. A stopped workload is one POST away from
// being deployable, and `up` means "make the file true", which for something
// that is not running means starting it, and then applying whatever else the
// file asks for onto the workload that is now up.
func deployable(live Live, workloadName, dirFlag string) error {
	switch live.State {
	case StateTerminated:
		return fmt.Errorf(
			"workload %s is terminated, which cannot be undone, and it still holds its name and artifact — "+
				"%s, then deploy again",
			workloadName, deleteRemedy(live.WorkloadID, dirFlag, true))

	case StateErrored:
		// The refusal carries its own exit. Ending at "check the logs" left
		// recovery to folklore, and the folk remedy — hand-deleting the
		// workloadId, or the whole state directory — either loops through the
		// name conflict below or throws away the code catalog with it.
		return fmt.Errorf(
			"workload %s is errored, so there is nothing safe to deploy onto. "+
				"%sIf it stays errored, %s, and deploy again to recreate it under the same name",
			workloadName, erroredCaveat(live.WorkloadID), deleteRemedy(live.WorkloadID, dirFlag, true))

	case StateSettling:
		// The backstop, not the answer. Every run waits a moving workload out
		// before the plan is built, so what reaches here is a workload that was
		// steady when it was read and is moving again by the time it is acted
		// on. The message describes rather than explains: this has no way to
		// know which of the two reads was the stale one.
		return fmt.Errorf(
			"workload %s is %s, which is not a state a deploy can act on. "+
				"Check 'dr workload status %s', then deploy",
			workloadName, live.Status, live.WorkloadID)

	case StateUnbound, StateMissing, StateRunning:
		return nil

	case StateStopped:
		return startable(live, workloadName)

	default:
		return nil
	}
}

// startable refuses the one status that cannot be started. The platform
// no-ops a start of a suspended workload, so accepting it would poll until
// the timeout for a transition that is never coming. Interrupted can be
// started.
func startable(live Live, workloadName string) error {
	if !workload.IsSuspendedWorkloadStatus(live.Status) {
		return nil
	}

	return fmt.Errorf(
		"workload %s is suspended, which a deploy cannot undo: the platform ignores a start while it is "+
			"in that state. Find out why it was suspended before deploying again",
		workloadName)
}

// create makes the workload and waits for it to serve. payload is the create
// request: the compiled manifest as it stands for a published image, or the
// same thing with the inline artifact swapped for the id of the one that was
// just built.
//
// The binding is written back the moment the workload exists, before anything
// else can fail, so a run that dies during the wait still leaves the next one
// able to find what it made.
func create(
	loaded Loaded,
	payload json.RawMessage,
	result Result,
	opts Options,
	report *reporter,
) (Result, error) {
	var created *workload.Workload

	err := report.run("Creating workload", func() error {
		wl, createErr := createWorkloadFn(payload)
		if createErr != nil {
			return nameTaken(createErr, result.Name, loaded.Path, dirFlagFor(loaded))
		}

		created = wl

		return nil
	})
	if err != nil {
		return result, err
	}

	result.WorkloadID = created.ID
	result.ArtifactID = created.ArtifactID
	result.Endpoint = created.Endpoint
	result.Status = created.Status
	result.Action = ActionCreated

	if err := writeWorkloadIDFn(loaded.Path, created.ID); err != nil {
		return result, fmt.Errorf(
			"workload %s was created but its id could not be written to %s. "+
				"Set 'workloadId: %s' by hand, replacing any workloadId already there, "+
				"or the next deploy will create a second workload: %w",
			created.ID, loaded.Path, created.ID, err)
	}

	if opts.Detach {
		report.say("  Workload %s requested; not waiting.\n", created.ID)

		return result, nil
	}

	return settle(created.ID, result, opts, report)
}

// startFirst brings a stopped workload up so the rest of the plan has
// something to land on, and is the whole apply when starting is all the plan
// asks for.
//
// Nothing is written back and nothing is compiled here, because nothing is
// being changed. The workload keeps the artifact it was stopped on, which is
// either the version this run just confirmed the file still describes, or the
// one the roll below is about to replace.
//
// --detach is honoured only for a start that is the whole run. When more
// follows, the wait is a prerequisite rather than the deploy: the rollout
// cannot be requested against a workload that is not up yet, and it is the
// rollout that --detach returns without waiting for. Saying so is better than
// silently blocking, so the note goes out before the wait rather than after it.
func startFirst(live Live, plan Plan, result Result, opts Options, report *reporter) (Result, error) {
	if err := requestStart(result.WorkloadID, report); err != nil {
		return result, err
	}

	result.Action = ActionStarted

	only := plan.OnlyStarts()

	if opts.Detach && only {
		// Without this the envelope reports "stopped" beside an action of
		// "started", which reads as a run that did nothing.
		result.Status = startingStatus(live.Status)

		report.say("  Workload %s start requested; not waiting.\n", result.WorkloadID)

		return result, nil
	}

	if opts.Detach {
		report.say("  Waiting for the start before deploying onto it; --detach applies to the deploy.\n")
	}

	// Only the run that ends here may lock: locking is about the artifact left
	// serving, and a start that is about to be rolled off is not that.
	if only {
		return settle(result.WorkloadID, result, opts, report)
	}

	return awaitRunning(result.WorkloadID, result, opts, report)
}

// requestStart POSTs the start and passes on the one thing the platform says
// about it.
func requestStart(workloadID string, report *reporter) error {
	var ack *workload.WorkloadOperationResponse

	err := report.run("Starting workload", func() error {
		response, startErr := startWorkloadFn(workloadID)
		ack = response

		return startErr
	})
	if err != nil {
		return fmt.Errorf("cannot start workload %s: %w", workloadID, err)
	}

	// The only place the platform says it did nothing: a start it no-ops comes
	// back 200 with a message saying so.
	if ack != nil && ack.Status != "" {
		report.say("  %s\n", ack.Status)
	}

	return nil
}

// startingStatus is what to report for a start nobody waited for. An
// unrecognised status is passed through rather than overwritten, and the
// recognised ones are matched the way every other status comparison here
// matches, which is without regard to case.
func startingStatus(was string) string {
	if workload.IsStoppedWorkloadStatus(was) {
		return workload.WorkloadStatusSubmitted
	}

	return was
}

// nameTaken turns a create conflict into an error naming what owns the name.
// A 409 says only that something of that name is already there; the id is what
// the user needs next and the one thing they cannot see from here.
//
// What this deliberately does not do is deploy onto it. Nothing tells "the
// workload this project made, whose id failed to be written back" apart from
// "an unrelated workload that happens to share a name", and the two want
// opposite things. Adopting the first is how a room full of people deploying
// the same template all bind to whoever ran it first, are told their own
// deploy succeeded, and under --lock permanently lock a stranger's artifact.
// The plan they were shown said a workload would be created; nothing about the
// file would have reached the one they were given.
//
// So the run stops and hands the choice back, which costs one line in the
// manifest in the case where adopting would have been right.
func nameTaken(createErr error, workloadName, path, dirFlag string) error {
	if !isConflict(createErr) || workloadName == "" {
		return createErr
	}

	existing, err := listWorkloadsFn(conflictSearchLimit, nil, "")
	if err != nil {
		// The conflict is the real story; a failure to look up what caused it
		// is a footnote that should not replace it.
		return createErr
	}

	for i := range existing {
		if existing[i].Name != workloadName {
			continue
		}

		// A workload a deploy cannot act on must not be recommended for
		// binding. Advising 'set workloadId' against an errored or terminated
		// holder sends the next run straight into that state's refusal — and
		// for the user who cleared the binding by hand because their workload
		// was stuck, it closes a loop: the 409 recommending the exact line
		// they just deleted. The advice is the one those refusals give:
		// delete what holds the name.
		if workload.IsWorkloadErrorStatus(existing[i].Status) {
			// Errored gets the same hedge deployable's own refusal carries;
			// terminated really is final, so it keeps the certainty.
			caveat := ""
			if !strings.EqualFold(existing[i].Status, workload.WorkloadStatusTerminated) {
				caveat = erroredCaveat(existing[i].ID)
			}

			return fmt.Errorf(
				"a workload named %s already exists (%s), is %s, and nothing was deployed onto it. "+
					"A deploy cannot act on it, so binding to it would only move this refusal. "+
					"%sIf it is this project's dead workload, %s "+
					"and deploy again to recreate the name. If it is not, rename this one in %s: %w",
				workloadName, existing[i].ID, strings.ToLower(existing[i].Status),
				caveat, deleteRemedy(existing[i].ID, dirFlag, false), path, createErr)
		}

		return fmt.Errorf(
			"a workload named %s already exists (%s) and nothing was deployed onto it. "+
				"If it is this project's, set 'workloadId: %s' in %s, replacing any already there, "+
				"and deploy again, which will print "+
				"what would change before changing it. If it is not, rename this one in %s: %w",
			workloadName, existing[i].ID, existing[i].ID, path, path, createErr)
	}

	return createErr
}

// deleteRemedy is the recovery every dead-workload refusal spells out, one
// implementation so the wording cannot drift between them. clearsBinding adds
// what the delete does to the manifest, which is only true when this project
// is bound to the workload being deleted — a create conflict has no binding
// to clear.
func deleteRemedy(workloadID, dirFlag string, clearsBinding bool) string {
	remedy := fmt.Sprintf("remove it with 'dr workload delete %s%s'", workloadID, dirFlag)
	if clearsBinding {
		remedy += ", which also clears the binding in " + manifest.FileName
	}

	return remedy
}

// erroredCaveat is the hedge every message about an errored workload carries:
// errored is the one dead-looking state that can still recover, so advising
// its deletion with certainty would have a slow starter deleted mid-start.
func erroredCaveat(workloadID string) string {
	return fmt.Sprintf(
		"Check 'dr workload logs %s' first; a slow start can report errored and then recover. ",
		workloadID)
}

// conflictSearchLimit bounds the lookup behind a create conflict. Past this
// many workloads a name scan is the wrong tool, and the conflict itself is
// still reported.
const conflictSearchLimit = 100

// isConflict reports whether err is the API's 409.
func isConflict(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict
}

// settle waits for the workload to come up, records where it landed, and locks
// the artifact serving when the run asked for that.
//
// It is the end of a run. A wait that happens partway through one, such as the
// start a roll onto a stopped workload needs before it can begin, calls
// awaitRunning instead: locking is about the artifact left serving, and an
// artifact about to be rolled off is not it. Locking cannot be undone, so the
// distinction is not a tidiness one.
func settle(workloadID string, result Result, opts Options, report *reporter) (Result, error) {
	result, err := awaitRunning(workloadID, result, opts, report)
	if err != nil || !opts.Lock {
		return result, err
	}

	return lock(result, report)
}

// awaitRunning waits for the workload to come up and records where it landed.
func awaitRunning(workloadID string, result Result, opts Options, report *reporter) (Result, error) {
	var final *workload.Workload

	err := report.run("Waiting for the workload to run", func() error {
		wl, waitErr := waitWorkloadFn(workloadID, opts.PollInterval, opts.PollTimeout, nil)
		final = wl

		return waitErr
	})

	if final != nil {
		result.Status = final.Status
		result.Endpoint = final.Endpoint
		result.ArtifactID = final.ArtifactID
	}

	if err != nil {
		return result, err
	}

	if workload.IsWorkloadErrorStatus(result.Status) {
		return result, fmt.Errorf("workload %s finished as %s; check 'dr workload logs %s'",
			workloadID, result.Status, workloadID)
	}

	return result, nil
}

// lock makes the live artifact permanent. It runs only after the workload is
// serving, because locking is one-way: locking an artifact that never came up
// would leave something undeletable behind and a workload that cannot be
// rolled off it.
func lock(result Result, report *reporter) (Result, error) {
	// Already locked, either by a roll onto locked production or before the
	// workload was stopped. Locking twice is not a no-op at the platform.
	if result.Locked {
		return result, nil
	}

	if result.ArtifactID == "" {
		return result, errors.New("cannot lock: the platform did not report which artifact is running")
	}

	err := report.run("Locking the artifact", func() error {
		_, lockErr := lockArtifactFn(result.ArtifactID)

		return lockErr
	})
	if err != nil {
		return result, fmt.Errorf("workload %s is running, but artifact %s could not be locked: %w",
			result.WorkloadID, result.ArtifactID, err)
	}

	result.Locked = true

	return result, nil
}

// defaultCodeChange measures the working tree against what the platform
// already holds, by asking the sync engine for its own plan.
//
// The engine acquires the project lock between planning and executing, so
// this closes it immediately: nothing here is going to sync, and holding the
// lock across a build would block the very command that comes next.
//
// DryRun is what makes this safe to ask of a locked artifact. The engine
// refuses to sync into one, and rightly, but this is a measurement rather than
// a sync: the count it produces is what tells the deploy to mint a new version
// and roll onto it. A refusal here would refuse that deploy, which is the only
// way past a lock.
func defaultCodeChange(loaded Loaded, _ Live) (change CodeChange, err error) {
	mode := loaded.Manifest.BuildMode()
	if mode != manifest.BuildModeDockerfile && mode != manifest.BuildModeGenerated {
		// A published image has no code to sync, and a file bound by artifact
		// id says nothing about how the image is made.
		return CodeChange{}, nil
	}

	// Read the ignore file here rather than off the engine below. The first
	// deploy of an unlinked project never builds one, and that is exactly the
	// run where someone who has just cloned a project carrying the old name
	// needs to hear it is going away.
	notice := ignoreFileNotice(loaded.ProjectDir)

	if !wapi.Exists(loaded.ProjectDir) {
		// Nothing has ever been synced from here, so everything is new.
		return CodeChange{Applies: true, FirstDeploy: true, IgnoreNotice: notice}, nil
	}

	engine, err := sync.New(loaded.ProjectDir, sync.Options{DryRun: true, Yes: true})
	if err != nil {
		return CodeChange{}, fmt.Errorf("cannot inspect the working tree: %w", err)
	}

	// A lock that will not release is not a detail to swallow: the build and
	// the sync that come next take the same lock, so the run would fail later
	// with a message about someone else holding it.
	defer func() {
		if closeErr := engine.Close(); closeErr != nil && err == nil {
			change, err = CodeChange{}, fmt.Errorf("cannot release the project lock: %w", closeErr)
		}
	}()

	plan, err := engine.Plan()
	if err != nil {
		return CodeChange{}, fmt.Errorf("cannot compare the working tree with the last deploy: %w", err)
	}

	return CodeChange{
		Applies:      true,
		Files:        len(plan.Uploads) + len(plan.Deletes),
		IgnoreNotice: notice,
		// The engine says so rather than this asking a second time: it fetched
		// the artifact to build the plan, and a locked one is the reason it
		// left a notice at all.
		LinkLocked: engine.LockedNotice() != "",
	}, nil
}

// ignoreFileNotice is the deprecation line for this project, empty when there
// is nothing to say.
//
// A read failure is not this function's problem: it is only deciding whether
// a sentence gets printed, and the sync that follows opens the same file and
// fails with a proper message if it cannot be read.
func ignoreFileNotice(projectDir string) string {
	m, err := ignore.New(projectDir)
	if err != nil {
		return ""
	}

	return m.Notice()
}
