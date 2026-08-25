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

	live, err := Look(loaded.WorkloadID())
	if err != nil {
		return Result{}, err
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

	result := Result{
		Plan:       plan,
		WorkloadID: live.WorkloadID,
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

	if err := Render(opts.Stderr, Summary{Name: result.Name, WorkloadID: result.WorkloadID}, plan); err != nil {
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
		return lockOnly(live, result, opts)
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
func lockOnly(live Live, result Result, opts Options) (Result, error) {
	if !opts.Lock || result.Locked {
		return result, nil
	}

	if err := deployable(live, result.Name); err != nil {
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
	if err := deployable(live, result.Name); err != nil {
		return result, err
	}

	// Starting is checked before the roll refusal so that a stopped workload
	// the file already agrees with gets deployed rather than turned away.
	// When the file asks for more than a start, the refusal wins: bringing the
	// workload up on the version it was stopped on would report a failure
	// having already changed something, which is the worst of both.
	if plan.Action() == ActionStarted {
		// A start is when the container resolves its references, so a
		// credential deleted while the workload was off would otherwise fail
		// minutes later as a container that will not come up.
		if err := verifyCredentials(loaded.Compiled.CredentialRefs, result.Name); err != nil {
			return result, err
		}

		return start(live, result, opts)
	}

	// Everything below this changes a workload that is running: a rollout
	// promotes onto something serving, and a settings change is followed by a
	// wait for the workload to come back. Starting first and then applying the
	// rest would bring the workload up on the version it was stopped on and
	// report a failure afterwards, which is worse than refusing.
	if live.State == StateStopped {
		return result, fmt.Errorf(
			"workload %s is stopped and the file asks for more than starting it. "+
				"Start it with 'dr workload start %s' and deploy again, so the change lands on "+
				"something that is running",
			result.Name, live.WorkloadID)
	}

	// A runtime-only change sends no environment at all: the artifact keeps
	// every variable it already has, so a credential this run does not touch
	// must not be able to refuse a resize.
	if !plan.Creates && !plan.RollsArtifact() {
		return retune(loaded, result, opts, newReporter(opts.Stderr, opts.Spinner))
	}

	// Last check before the first mutation, and the only one that needs the
	// network: a credential id is the one thing the local ledger cannot judge.
	if err := verifyCredentials(loaded.Compiled.CredentialRefs, result.Name); err != nil {
		return result, err
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	// A workload that already exists is replaced rather than created: the
	// endpoint has to survive, and something is serving on it meanwhile.
	if plan.RollsArtifact() {
		return roll(loaded, live, plan, result, opts, report)
	}

	// A published image is one POST. Anything the platform builds has to be
	// given somewhere to put the code and time to turn it into an image
	// first, which is a different shape of deploy rather than a longer one.
	if plan.Code.Applies {
		return buildAndCreate(loaded, plan.Code, result, opts, report)
	}

	return create(loaded, loaded.Compiled.Payload, result, opts, report)
}

// deployable refuses the live states that cannot take a deploy, one message
// each. None of them are guesses: a workload that is gone stays gone, and one
// that is unsettled may still be on its way somewhere.
//
// Stopped is not among them. A stopped workload is one POST away from being
// deployable, and `up` means "make the file true", which for something that
// is not running means starting it.
func deployable(live Live, workloadName string) error {
	switch live.State {
	case StateMissing, StateTerminated:
		return fmt.Errorf(
			"the workload this manifest is bound to is %s. "+
				"Bind to a live one with 'dr workload config --workload-id <id>', or remove workloadId from %s "+
				"to create a new workload on the next deploy",
			live.State, manifest.FileName)

	case StateErrored:
		return fmt.Errorf(
			"workload %s is errored, so there is nothing safe to deploy onto. "+
				"Check 'dr workload logs' first; a slow start can report errored and then recover",
			workloadName)

	case StateSettling:
		return fmt.Errorf(
			"workload %s is still settling. Wait for it to finish, then deploy",
			workloadName)

	case StateUnbound, StateRunning:
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
	if live.Status != workload.WorkloadStatusSuspended {
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
			return nameTaken(createErr, result.Name, loaded.Path)
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
				"Add 'workloadId: %s' by hand, or the next deploy will create a second workload: %w",
			created.ID, loaded.Path, created.ID, err)
	}

	if opts.Detach {
		report.say("  Workload %s requested; not waiting.\n", created.ID)

		return result, nil
	}

	return settle(created.ID, result, opts, report)
}

// start brings a stopped workload back up. It is the whole apply for a plan
// that found nothing else to change: the file and the live spec already
// agree, and the only thing standing between them and a served request is
// that the workload is off.
//
// Nothing is written back and nothing is compiled here, because nothing is
// being changed. The workload keeps the artifact it was stopped on, which is
// the version this run just confirmed the file still describes.
func start(live Live, result Result, opts Options) (Result, error) {
	report := newReporter(opts.Stderr, opts.Spinner)

	var ack *workload.WorkloadOperationResponse

	err := report.run("Starting workload", func() error {
		response, startErr := startWorkloadFn(result.WorkloadID)
		ack = response

		return startErr
	})
	if err != nil {
		return result, fmt.Errorf("cannot start workload %s: %w", result.WorkloadID, err)
	}

	// The only place the platform says it did nothing: a start it no-ops comes
	// back 200 with a message saying so.
	if ack != nil && ack.Status != "" {
		report.say("  %s\n", ack.Status)
	}

	result.Action = ActionStarted

	if opts.Detach {
		// Without this the envelope reports "stopped" beside an action of
		// "started", which reads as a run that did nothing.
		result.Status = startingStatus(live.Status)

		report.say("  Workload %s start requested; not waiting.\n", result.WorkloadID)

		return result, nil
	}

	return settle(result.WorkloadID, result, opts, report)
}

// startingStatus is what to report for a start nobody waited for. An
// unrecognised status is passed through rather than overwritten.
func startingStatus(was string) string {
	switch was {
	case workload.WorkloadStatusStopped,
		workload.WorkloadStatusSuspended,
		workload.WorkloadStatusInterrupted:
		return workload.WorkloadStatusSubmitted

	default:
		return was
	}
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
func nameTaken(createErr error, workloadName, path string) error {
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

		return fmt.Errorf(
			"a workload named %s already exists (%s) and nothing was deployed onto it. "+
				"If it is this project's, add 'workloadId: %s' to %s and deploy again, which will print "+
				"what would change before changing it. If it is not, rename this one in %s: %w",
			workloadName, existing[i].ID, existing[i].ID, path, path, createErr)
	}

	return createErr
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

// settle waits for the workload to come up and records where it landed.
func settle(workloadID string, result Result, opts Options, report *reporter) (Result, error) {
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

	if !opts.Lock {
		return result, nil
	}

	return lock(result, report)
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
