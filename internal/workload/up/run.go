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
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
)

// Test seams. The deploy is mostly network, and the tests are not.
var (
	runWizardFn       = wizard.Run
	createWorkloadFn  = workload.CreateWorkload
	waitWorkloadFn    = workload.WaitForWorkload
	listWorkloadsFn   = workload.ListWorkloads
	startWorkloadFn   = workload.StartWorkload
	createArtifactFn  = workload.CreateArtifact
	getArtifactFn     = workload.GetArtifact
	lockArtifactFn    = workload.LockArtifact
	triggerBuildFn    = workload.TriggerArtifactBuild
	waitBuildFn       = workload.WaitForBuild
	listBuildsFn      = workload.ListArtifactBuilds
	getCredentialFn   = workload.GetCredential
	writeWorkloadIDFn = manifest.WriteWorkloadID
	codeChangeFn      = defaultCodeChange
	projectLinkedFn   = wapi.Exists
	loadProjectFn     = wapi.LoadConfig
	initProjectFn     = wapi.Initialize
	syncProjectFn     = defaultSync
)

// ErrNotWired reports a path the plan understands but the apply cannot take
// yet. It is a sentinel so the command can say so in its own words, and so a
// test can tell "we refused" from "we broke".
var ErrNotWired = errors.New("not wired yet")

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

	// Lock makes the artifact that ends up live immutable and permanent.
	// Locking is one-way, so it happens last, only after the workload is
	// actually serving: locking something that never came up would leave an
	// undeletable artifact behind.
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
		Action:     plan.Action(),
		Locked:     live.Locked,
	}

	if err := Render(opts.Stderr, Summary{Name: result.Name, WorkloadID: result.WorkloadID}, plan); err != nil {
		return result, err
	}

	noteUnusedForce(plan, opts)

	if opts.DryRun || plan.Empty() {
		return result, nil
	}

	return apply(loaded, live, plan, result, opts)
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

	// Checked before every path below, which either mutates or refuses. A
	// start counts as a mutation: it is when the container resolves its
	// references, so a credential deleted while the workload was off would
	// surface minutes later as a container that will not come up. On a path
	// about to be refused it shadows ErrNotWired, deliberately: one run to
	// learn about both beats two.
	if err := verifyCredentials(loaded.Compiled.CredentialRefs); err != nil {
		return result, err
	}

	// Starting is checked before the roll refusal so that a stopped workload
	// the file already agrees with gets deployed rather than turned away.
	// When the file asks for more than a start, the refusal wins: bringing the
	// workload up on the version it was stopped on would report a failure
	// having already changed something, which is the worst of both.
	if plan.Action() == ActionStarted {
		return start(live, result, opts)
	}

	if !plan.Creates {
		return result, fmt.Errorf("%w: %s. The plan above is correct; applying it is the next change",
			ErrNotWired, unwired(plan))
	}

	report := newReporter(opts.Stderr, opts.Spinner)

	// A published image is one POST. Anything the platform builds has to be
	// given somewhere to put the code and time to turn it into an image
	// first, which is a different shape of deploy rather than a longer one.
	if plan.Code.Applies {
		return buildAndCreate(loaded, plan.Code, result, opts, report)
	}

	return create(loaded, loaded.Compiled.Payload, result, opts, report)
}

// unwired names the change the plan asks for. Calling every live workload a
// roll is wrong for a runtime-only plan, which mints no artifact: the printed
// plan and the refusal underneath it then disagree about what is missing.
func unwired(plan Plan) string {
	// Calling a stopped workload "live" contradicts the plan block above,
	// whose first line says it is to be started.
	subject := "a live workload"
	if plan.State == StateStopped {
		subject = "a stopped workload, which is also not being started"
	}

	if plan.Action() == ActionUpdated {
		return "changing the runtime settings of " + subject
	}

	return "rolling " + subject + " onto a new version"
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

	existing, err := listWorkloadsFn(conflictSearchLimit, nil)
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
	// Already locked, and locking twice is not a no-op at the platform.
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
func defaultCodeChange(loaded Loaded, _ Live) (change CodeChange, err error) {
	mode := loaded.Manifest.BuildMode()
	if mode != manifest.BuildModeDockerfile && mode != manifest.BuildModeGenerated {
		// A published image has no code to sync, and a file bound by artifact
		// id says nothing about how the image is made.
		return CodeChange{}, nil
	}

	if !wapi.Exists(loaded.ProjectDir) {
		// Nothing has ever been synced from here, so everything is new.
		return CodeChange{Applies: true, FirstDeploy: true}, nil
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

	return CodeChange{Applies: true, Files: len(plan.Uploads) + len(plan.Deletes)}, nil
}
