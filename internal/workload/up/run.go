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
	lockArtifactFn    = workload.LockArtifact
	writeWorkloadIDFn = manifest.WriteWorkloadID
	codeChangeFn      = defaultCodeChange
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

	if opts.DryRun || plan.Empty() {
		return result, nil
	}

	return apply(loaded, live, plan, result, opts)
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

	if !plan.Creates {
		return result, fmt.Errorf("%w: %s. The plan above is correct; applying it is the next change",
			ErrNotWired, unwired(plan))
	}

	// Only an image the platform builds needs a version, a sync and a build
	// before the create. A published imageUri and a file bound to an existing
	// artifact id are the same single POST, so neither is refused here.
	if mode := loaded.Manifest.BuildMode(); mode == manifest.BuildModeDockerfile || mode == manifest.BuildModeGenerated {
		return result, fmt.Errorf(
			"%w: creating a workload whose image the platform builds (%s mode). "+
				"Publishing the image yourself and switching the manifest to an imageUri works today",
			ErrNotWired, mode)
	}

	return create(loaded, result, opts)
}

// unwired names the change the plan asks for. Calling every live workload a
// roll is wrong for a runtime-only plan, which mints no artifact: the printed
// plan and the refusal underneath it then disagree about what is missing.
func unwired(plan Plan) string {
	if plan.Action() == ActionUpdated {
		return "changing the runtime settings of a live workload"
	}

	return "rolling a live workload onto a new version"
}

// deployable refuses the live states that cannot take a deploy, one message
// each. None of them are guesses: a workload that is gone stays gone, and one
// that is unsettled may still be on its way somewhere.
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

	case StateStopped:
		return fmt.Errorf(
			"%w: starting a stopped workload before deploying onto it", ErrNotWired)

	case StateUnbound, StateRunning:
		return nil

	default:
		return nil
	}
}

// create makes the workload from the compiled manifest and waits for it to
// serve. The binding is written back the moment the workload exists, before
// anything else can fail, so a run that dies during the wait still leaves the
// next one able to find what it made.
func create(loaded Loaded, result Result, opts Options) (Result, error) {
	report := newReporter(opts.Stderr, opts.Spinner)

	var created *workload.Workload

	err := report.run("Creating workload", func() error {
		wl, err := createWorkloadFn(loaded.Compiled.Payload)
		if err != nil {
			adopted, adoptErr := adopt(err, result.Name)
			if adoptErr != nil {
				return adoptErr
			}

			wl = adopted
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

// adopt turns a create conflict into the workload that caused it. A 409 means
// something of that name is already there, and failing would leave the user
// with a name they cannot deploy and no way to reach what owns it.
func adopt(createErr error, workloadName string) (*workload.Workload, error) {
	if !isConflict(createErr) || workloadName == "" {
		return nil, createErr
	}

	existing, err := listWorkloadsFn(adoptSearchLimit, nil)
	if err != nil {
		// The conflict is the real story; a failure to look up what caused it
		// is a footnote that should not replace it.
		return nil, createErr
	}

	for i := range existing {
		if existing[i].Name == workloadName {
			return &existing[i], nil
		}
	}

	return nil, createErr
}

// adoptSearchLimit bounds the lookup behind a create conflict. Past this many
// workloads a name scan is the wrong tool, and the conflict itself is still
// reported.
const adoptSearchLimit = 100

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

// Credentials are not verified before the deploy yet. Compile hands back
// every dr-credential reference precisely so each can be checked with one GET
// before anything mutates, which turns a mistyped id from a container that
// will not start into an error printed in milliseconds. The lookup itself
// lives with the other workload API clients and reaches this stack only once
// that change is merged; wiring it is a few lines over Compiled.CredentialRefs
// at the top of Run.
