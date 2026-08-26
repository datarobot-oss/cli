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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unboundImageManifest deploys a published image and has never been deployed,
// which is the one apply path this change can actually take.
const unboundImageManifest = `name: my-app
importance: low
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// unboundDockerfileManifest asks the platform to build, which is the path
// that is planned but not yet applied.
const unboundDockerfileManifest = `name: my-app
importance: low
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageBuildConfig:
              dockerfile:
                source: provided
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// linkedArtifactDoc is the artifact unboundDockerfileManifest describes, in the
// shape the comparison expects to find it. A test that leaves this out is not
// testing reuse: the failed read folds into "reuse it" and the comparison never
// runs, so the assertion passes for the wrong reason.
//
// The kind sits inside the spec because that is where the manifest above puts
// it, and the two sides have to agree for a match. A real platform hoists it to
// the document root, so a manifest spelling it here and an artifact returning it
// there never match and the project mints an artifact per deploy. That is not
// this change's to fix; the fixture mirrors the manifest so these tests measure
// what they say they measure.
func linkedArtifactDoc(id string) (workload.Document, error) {
	return workload.Document{
		"id":     id,
		"status": workload.ArtifactStatusDraft,
		"spec": map[string]any{
			"type": "service",
			"containerGroups": []any{map[string]any{
				"name": "default",
				"containers": []any{map[string]any{
					"name":    "primary",
					"primary": true,
					"port":    float64(8080),
					"imageBuildConfig": map[string]any{
						"dockerfile": map[string]any{"source": "provided"},
					},
				}},
			}},
		},
	}, nil
}

// boundArtifactManifest names an artifact the platform already holds instead
// of describing one. There is no container to read a build mode from, so the
// mode is empty, and the create is the same single POST as a published image.
const boundArtifactManifest = `name: my-app
importance: low
artifactId: 68b0bbbb0000000000000002
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// unboundCredentialManifest names a stored credential, which is the one thing
// in the file no local check can judge.
const unboundCredentialManifest = `name: my-app
importance: low
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars:
              - name: OPENAI_API_KEY
                value: dr-credential:68b0cccc0000000000000003/apiToken
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// fakes collects the seams a run touches. A seam a test does not wire is
// replaced by one that fails the test if it is reached, so a run can never
// reach the network by omission and a test only wires what it means to
// exercise.
type fakes struct {
	wizard         func(wizard.Options) (wizard.Result, error)
	create         func(any) (*workload.Workload, error)
	wait           func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
	waitSteady     func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
	list           func(int, []string, string) ([]workload.Workload, error)
	start          func(string) (*workload.WorkloadOperationResponse, error)
	lock           func(string) (*workload.Artifact, error)
	cred           func(string) (*workload.Credential, error)
	findCredential func(string, int) (*workload.Credential, error)
	writeID        func(string, string) error
	code           func(Loaded, Live) (CodeChange, error)
	workloadD      func(string) (workload.Document, error)
	artifactD      func(string) (workload.Document, error)

	// The build track: create an artifact, link the project to it, push the
	// code, turn it into an image.
	newArtifact func(any) (*workload.Artifact, error)
	getArtifact func(string) (*workload.Artifact, error)
	linked      func(string) bool
	project     func(string) (wapi.Config, error)
	link        func(string, wapi.InitOptions) error
	save        func(string, wapi.Config) error
	codeRef     func(string, string, string) error
	sync        func(string) (*sync.Result, error)
	build       func(string) (*workload.BuildTriggerResponse, error)
	waitBuild   func(string, string, time.Duration, time.Duration, func(*workload.Build)) (*workload.Build, error)
	builds      func(string, int) ([]workload.Build, error)

	// checkEndpoint is the one GET a deploy ends with.
	checkEndpoint func(string) (int, error)

	// The roll track: refuse to queue a second swap, start one, follow it.
	guard       func(string) error
	replace     func(string, string, json.RawMessage) (*workload.Replacement, error)
	waitReplace func(string, *workload.Replacement, time.Duration, time.Duration,
		func(*workload.Replacement)) (*workload.Replacement, error)

	// settings is the in-place path: a change that moved only the sizing.
	settings func(string, json.RawMessage) (*workload.Replacement, error)
}

// install swaps in the seams the test supplied and restores them afterwards.
func install(t *testing.T, f fakes) {
	t.Helper()

	// Two seams default to doing nothing rather than to the real thing. A
	// test that cares about neither the write-back nor the working tree
	// should not have to say so, and neither may touch a real project.
	force(t, &writeWorkloadIDFn, func(string, string) error { return nil })
	force(t, &codeChangeFn, func(Loaded, Live) (CodeChange, error) { return CodeChange{}, nil })

	// The endpoint check would otherwise GET whatever URL the fixture
	// carries, spending a real network timeout per test that reaches settle.
	force(t, &checkEndpointFn, func(string) (int, error) { return http.StatusOK, nil })
	swap(t, &checkEndpointFn, f.checkEndpoint)

	// swap leaves a seam alone when the test supplied nothing, so without this
	// a stopped fixture with no start fake POSTs to whatever tenant the
	// developer is logged into.
	force(t, &startWorkloadFn, func(id string) (*workload.WorkloadOperationResponse, error) {
		t.Fatalf("the run started workload %s, which this test did not wire", id)

		return nil, nil
	})

	// The same for the wait a settling workload triggers, which a fixture can
	// reach without the test having asked for it.
	force(t, &waitSteadyFn,
		func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			t.Fatalf("the run waited on workload %s to settle, which this test did not wire", id)

			return nil, nil
		})

	swap(t, &waitSteadyFn, f.waitSteady)

	// The placeholder message looks a credential up by name. Left unwired it
	// would list credentials from whatever tenant the developer is logged
	// into; answering "no such credential" is the quiet default every test
	// that does not care about the lookup wants.
	force(t, &findCredentialFn, func(string, int) (*workload.Credential, error) { return nil, nil })
	swap(t, &findCredentialFn, f.findCredential)

	swap(t, &runWizardFn, f.wizard)
	swap(t, &createWorkloadFn, f.create)
	swap(t, &waitWorkloadFn, f.wait)
	swap(t, &listWorkloadsFn, f.list)
	swap(t, &startWorkloadFn, f.start)
	swap(t, &lockArtifactFn, f.lock)
	swap(t, &getCredentialFn, f.cred)
	swap(t, &writeWorkloadIDFn, f.writeID)
	swap(t, &codeChangeFn, f.code)
	swap(t, &getWorkloadDocFn, f.workloadD)
	swap(t, &getArtifactDocFn, f.artifactD)

	// A test that does not wire the build track must not be able to reach it
	// by accident, and an unlinked project is the state a first deploy starts
	// from.
	force(t, &projectLinkedFn, func(string) bool { return false })

	// Unlike the seams below it, this one is reached by the lock path as well
	// as the build track, so leaving it unset would send a test that gets
	// there to whatever tenant the developer is logged into.
	force(t, &getArtifactFn, func(id string) (*workload.Artifact, error) {
		t.Fatalf("the run read artifact %s, which this test did not wire", id)

		return nil, nil
	})

	swap(t, &createArtifactFn, f.newArtifact)
	swap(t, &getArtifactFn, f.getArtifact)
	swap(t, &projectLinkedFn, f.linked)
	swap(t, &loadProjectFn, f.project)
	swap(t, &initProjectFn, f.link)
	swap(t, &saveProjectFn, f.save)
	swap(t, &patchCodeRefFn, f.codeRef)
	swap(t, &syncProjectFn, f.sync)
	swap(t, &triggerBuildFn, f.build)
	swap(t, &waitBuildFn, f.waitBuild)
	swap(t, &listBuildsFn, f.builds)

	// Nothing stands in the way of a rollout unless a test says so, because
	// the quiet answer is the one every other roll test wants.
	force(t, &guardReplacementFn, func(string) error { return nil })
	swap(t, &guardReplacementFn, f.guard)
	swap(t, &startReplacementFn, f.replace)
	swap(t, &waitReplacementFn, f.waitReplace)
	swap(t, &updateSettingsFn, f.settings)
}

// swap installs fake over the seam at target, and does nothing when the test
// did not supply one. reflect is what makes that distinction possible: a nil
// func in a type parameter is still a value, so there is no way to compare it
// against nil directly.
func swap[F any](t *testing.T, target *F, fake F) {
	t.Helper()

	if reflect.ValueOf(fake).IsNil() {
		return
	}

	force(t, target, fake)
}

// force installs value unconditionally, restoring what was there when the
// test ends.
func force[F any](t *testing.T, target *F, value F) {
	t.Helper()

	prev := *target
	*target = value

	t.Cleanup(func() { *target = prev })
}

// runIn deploys the manifest written into a fresh directory.
func runIn(t *testing.T, content string, opts Options) (Result, string, error) {
	t.Helper()

	dir := t.TempDir()
	writeManifest(t, dir, content)

	// The validator refuses a provided-source build with no Dockerfile beside
	// the manifest, which is correct and means the build-track fixtures need
	// a real one. Writing it unconditionally is harmless for the others.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM scratch\nEXPOSE 8080\n"), 0o600))

	var stderr bytes.Buffer

	opts.Dir = dir
	opts.Stderr = &stderr

	result, err := Run(opts)

	return result, stderr.String(), err
}

func running(id string) *workload.Workload {
	return &workload.Workload{
		ID: id, Name: "my-app", Status: workload.WorkloadStatusRunning,
		ArtifactID: "art-1", Endpoint: "https://app.datarobot.com/workloads/" + id + "/",
	}
}

// TestRun_NoManifestWithoutATerminalNamesTheFix is the rule that keeps a CI
// job from deploying a workload nobody described.
func TestRun_NoManifestWithoutATerminalNamesTheFix(t *testing.T) {
	install(t, fakes{
		wizard: func(wizard.Options) (wizard.Result, error) {
			t.Fatal("the wizard must not run without a terminal")

			return wizard.Result{}, nil
		},
	})

	var stderr bytes.Buffer

	_, err := Run(Options{Dir: t.TempDir(), NonInteractive: true, Stderr: &stderr})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoManifest)
	assert.Contains(t, err.Error(), "dr workload config")
}

// TestRun_NoManifestOnATerminalRunsTheWizard: setup and the first deploy are
// one act for someone standing at a keyboard.
func TestRun_NoManifestOnATerminalRunsTheWizard(t *testing.T) {
	dir := t.TempDir()

	var asked bool

	install(t, fakes{
		wizard: func(opts wizard.Options) (wizard.Result, error) {
			asked = true

			writeManifest(t, dir, unboundImageManifest)

			return wizard.Result{Path: opts.Dir}, nil
		},
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	var stderr bytes.Buffer

	result, err := Run(Options{Dir: dir, Stderr: &stderr})
	require.NoError(t, err)
	assert.True(t, asked)
	assert.Equal(t, "wl-new", result.WorkloadID)
}

func TestRun_DryRunAppliesNothing(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			t.Fatal("a dry run must not create anything")

			return nil, nil
		},
	})

	result, stderr, err := runIn(t, unboundImageManifest, Options{DryRun: true, NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, ActionCreated, result.Action)
	assert.Empty(t, result.WorkloadID)
	assert.Contains(t, stderr, "+ workload")
}

// TestRun_AlreadyUpToDate exits without touching anything, which is what
// makes `up` cheap to run on every push.
func TestRun_AlreadyUpToDate(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		create: func(any) (*workload.Workload, error) {
			t.Fatal("nothing changed, so nothing should be created")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr, "Already up to date")
}

// --lock is about the end state, not about what this run changed, so a run
// that finds nothing to do still has to make the serving artifact permanent.
// Returning early on an empty plan printed "Already up to date" and exited 0
// having locked nothing, which silently broke the sequence the draft warning
// asks for: deploy, read the warning, run 'up --lock'. By then the plan is
// always empty.
func TestRun_LockWithNothingToDoStillLocks(t *testing.T) {
	locked := ""

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return draftArtifact(t), nil },
		lock: func(id string) (*workload.Artifact, error) {
			locked = id

			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.NotEmpty(t, locked, "the artifact that is serving is the one --lock is about")
	assert.Equal(t, result.ArtifactID, locked)
	assert.True(t, result.Locked)
}

// The lock a `--lock` run takes on an empty plan lands on whatever Look read as
// serving, and a rollout in flight is about to move the workload off exactly
// that artifact. Locking cannot be undone, so a lost race would leave the
// outgoing version permanent and the rollout unable to complete. deployable
// cannot catch it: the workload reports itself running for the whole of a swap.
func TestRun_LockWithNothingToDoWaitsForARolloutInFlight(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return draftArtifact(t), nil },
		guard: func(workloadID string) error {
			return fmt.Errorf("workload %s: %w (status switching)",
				workloadID, workload.ErrReplacementInFlight)
		},
		lock: func(string) (*workload.Artifact, error) {
			t.Fatal("locking is one-way, so it may not land on a version being rolled off")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, _, err := runIn(t, bound, Options{NonInteractive: true, Lock: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a replacement is already in progress")
}

// The same run without the flag stays as cheap as it was. Locking is one-way,
// so an empty plan must never take it on its own initiative.
func TestRun_NothingToDoWithoutLockLocksNothing(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return draftArtifact(t), nil },
		lock: func(string) (*workload.Artifact, error) {
			t.Fatal("nothing asked for a lock")

			return nil, nil
		},
		// Recorded rather than left to install's silent no-op. The guard is
		// justified by not spending a round trip on the quiet path, and this is
		// the quietest path there is: an empty plan that is not locking has
		// nothing to guard, so a guard appearing here would be pure cost and
		// the default fake would hide it.
		guard: func(string) error {
			t.Fatal("a run that changes nothing has no rollout to guard against")

			return nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.False(t, result.Locked)
}

// boundLiveManifest says exactly what the live fixture already says, so the
// plan comes out empty.
const boundLiveManifest = `name: gpt-oss-20b-vllm
importance: moderate
artifact:
  name: gpt-oss-20b-vllm-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: vllm-server
            primary: true
            port: 8000
            imageBuildConfig:
              dockerfile:
                source: provided
runtime:
  containerGroups:
    - name: default
      containers:
        - name: vllm-server
          resourceAllocation: {cpu: 3, memory: 22GB, gpu: 1}
`

func TestRun_CreatesFromAPublishedImage(t *testing.T) {
	var (
		payload              any
		writtenTo, writtenID string
	)

	install(t, fakes{
		create: func(p any) (*workload.Workload, error) {
			payload = p

			return running("wl-new"), nil
		},
		writeID: func(path, id string) error {
			writtenTo, writtenID = path, id

			return nil
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	result, stderr, err := runIn(t, unboundImageManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.NotNil(t, payload)
	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Equal(t, ActionCreated, result.Action)
	assert.Equal(t, workload.WorkloadStatusRunning, result.Status)
	assert.Contains(t, result.Endpoint, "wl-new")
	assert.Equal(t, "wl-new", writtenID, "the binding is written back so the next run finds it")
	assert.Contains(t, writtenTo, ".datarobot.yaml")
	assert.Contains(t, stderr, "✓ Creating workload")
}

// TestRun_WriteBackFailureTellsTheUserHowToRecover is the difference between
// a recoverable hiccup and a duplicate workload on the next deploy.
func TestRun_WriteBackFailureTellsTheUserHowToRecover(t *testing.T) {
	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return errors.New("disk full") },
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "wl-new")
	assert.Contains(t, err.Error(), "by hand")
	assert.Equal(t, "wl-new", result.WorkloadID, "the id has to survive the error or the workload is unfindable")
}

// A name that is taken is reported with the id that owns it, and nothing
// else happens. Deploying onto it instead would bind this project to a
// workload nothing in the plan described, and report the create that never
// happened as a success.
func TestRun_ConflictNamesTheWorkloadThatOwnsTheName(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		list: func(int, []string, string) ([]workload.Workload, error) {
			return []workload.Workload{{ID: "wl-other", Name: "someone-else"}, *running("wl-existing")}, nil
		},
		writeID: func(string, string) error {
			t.Fatal("a workload this run did not create must not be bound to")

			return nil
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			t.Fatal("nothing was deployed, so there is nothing to wait for")

			return nil, nil
		},
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, Lock: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "wl-existing", "the id is what the user cannot see from here")
	assert.Contains(t, err.Error(), "workloadId: wl-existing")
	assert.Contains(t, err.Error(), ".datarobot.yaml")
	assert.Empty(t, result.WorkloadID, "reporting an id would say this run reached that workload")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "the platform's own refusal survives")
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func TestRun_ConflictWithNoMatchKeepsTheOriginalError(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		list: func(int, []string, string) ([]workload.Workload, error) { return nil, errors.New("list unavailable") },
	})

	_, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true})
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "a failed lookup must not replace the conflict that caused it")
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func TestRun_DetachSkipsTheWait(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			t.Fatal("--detach must not wait")

			return nil, nil
		},
	})

	result, stderr, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)
	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Contains(t, stderr, "not waiting")
}

// TestRun_LockHappensAfterTheWorkloadServes: locking is one-way, so locking
// something that never came up would leave an undeletable artifact behind.
func TestRun_LockHappensAfterTheWorkloadServes(t *testing.T) {
	var order []string

	install(t, fakes{
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			order = append(order, "wait")

			return running("wl-new"), nil
		},
		lock: func(id string) (*workload.Artifact, error) {
			order = append(order, "lock:"+id)

			return &workload.Artifact{ID: id}, nil
		},
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"wait", "lock:art-1"}, order)
	assert.True(t, result.Locked)
}

func TestRun_LockIsNotAttemptedWhenTheWorkloadFailed(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			errored := running("wl-new")
			errored.Status = workload.WorkloadStatusErrored

			return errored, nil
		},
		lock: func(string) (*workload.Artifact, error) {
			t.Fatal("a workload that did not come up must not have its artifact locked")

			return nil, nil
		},
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, Lock: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr workload logs")
	assert.False(t, result.Locked)
}

// TestRun_RefusesTheStatesThatCannotTakeADeploy checks each live state gets
// its own message rather than a generic failure.
//
// The settling statuses are not among them any more: a workload still moving is
// waited out before the plan is built, so it is deployed rather than refused.
func TestRun_RefusesTheStatesThatCannotTakeADeploy(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{workload.WorkloadStatusErrored, "dr workload logs"},
	}

	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			install(t, fakes{
				workloadD: func(string) (workload.Document, error) {
					return workload.Document{"id": "wl-1", "name": "my-app", "status": c.status}, nil
				},
				artifactD: func(string) (workload.Document, error) { return nil, nil },
				create: func(any) (*workload.Workload, error) {
					t.Fatal("nothing may be created for a workload in this state")

					return nil, nil
				},
			})

			bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

			_, _, err := runIn(t, bound, Options{NonInteractive: true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

// The reported dead end: a workload mid-transition used to be refused with
// "wait for it to finish, then deploy", which is work the command can do
// itself. It waits, re-reads, and deploys against where the workload landed.
func TestRun_SettlingWorkloadIsWaitedOutAndThenDeployed(t *testing.T) {
	var (
		looks  int
		waited string
	)

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			looks++

			d := stoppedWorkload(t)
			if looks == 1 {
				d["status"] = workload.WorkloadStatusProvisioning
			}

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		waitSteady: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			waited = id

			return &workload.Workload{ID: id, Status: workload.WorkloadStatusStopped}, nil
		},
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", waited)
	assert.Equal(t, 2, looks, "the workload is re-read, because a transition can land somewhere new")
	assert.Equal(t, ActionStarted, result.Action,
		"the plan is built against where it settled, which here is stopped")
	assert.Contains(t, stderr, "Waiting for the workload to settle")
}

// A workload still moving after the wait gave up has not been deployed onto.
// The message names where it got to, because "still stopping after 30m" is what
// decides whether to wait longer or go and look at the platform.
func TestRun_SettlingThatNeverLandsStopsBeforeMutating(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			d := stoppedWorkload(t)
			d["status"] = workload.WorkloadStatusStopping

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		waitSteady: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: id, Status: workload.WorkloadStatusStopping},
				errors.New("timeout waiting for workload " + id + " after 30m0s")
		},
		create: func(any) (*workload.Workload, error) {
			t.Fatal("a run that never got a settled state must not have deployed")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was still stopping")
	assert.Contains(t, err.Error(), "dr workload status 68b0c1d2e3f4a5b6c7d8e9f0")

	// The binding survives the failure. Without it the command prints no JSON
	// envelope at all, and losing the id is how a deploy becomes unfindable.
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", result.WorkloadID)
}

// A workload deleted while the deploy was waiting it out is drift, not a wait
// that failed. Reporting "did not settle" against an id nothing answers to
// would send the reader to a status command that 404s, when the answer `up`
// already has for a workload that is gone is to create one.
func TestRun_WorkloadDeletedDuringTheWaitIsRecreated(t *testing.T) {
	var looks int

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			looks++
			if looks > 1 {
				return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
			}

			d := stoppedWorkload(t)
			d["status"] = workload.WorkloadStatusStopping

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		waitSteady: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Equal(t, ActionCreated, result.Action)
}

// --detach is about not waiting for the deploy to serve, and this wait is
// before the deploy: what to apply cannot be known until the workload stops
// moving. Blocking is right, blocking in silence is not.
func TestRun_DetachedRunSaysWhyItIsWaitingToSettle(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		waitSteady: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: id, Status: workload.WorkloadStatusStopped}, nil
		},
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
	})

	// stoppedWorkload is read once as stopping so the wait fires, and the
	// re-read is the stopped fixture the seam above already returns.
	first := true

	force(t, &getWorkloadDocFn, func(string) (workload.Document, error) {
		d := stoppedWorkload(t)

		if first {
			first = false
			d["status"] = workload.WorkloadStatusStopping
		}

		return d, nil
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, stderr, err := runIn(t, bound, Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "--detach applies to the deploy")
}

// A preview must not block for the poll timeout: it changes nothing, so the
// honest answer is the plan as things stand plus a note that a deploy would
// wait. The header carries the state, so nothing here reads as settled.
func TestRun_DryRunOnASettlingWorkloadDoesNotWait(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			d := stoppedWorkload(t)
			d["status"] = workload.WorkloadStatusLaunching

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "plan against where it lands")
	assert.Contains(t, stderr, "settling")
	assert.Equal(t, "settling", result.Status)

	// The trap this closes: nothing differs from the state as it stands, so the
	// preview used to claim the deploy would do nothing, while the deploy that
	// follows waits for the stop to land and then starts the workload.
	assert.NotContains(t, stderr, "Already up to date")
}

// A workload that dies while the deploy is watching it must not be reported as
// a success. The deploy now waits transitions out, so a workload that settles
// into errored is one this run had a front-row seat to, and the file matching
// it field for field is not a reason to print a green tick over the wreckage.
func TestRun_SettlingIntoErroredIsNotReportedAsUpToDate(t *testing.T) {
	for _, status := range []string{workload.WorkloadStatusErrored, workload.WorkloadStatusTerminated} {
		t.Run(status, func(t *testing.T) {
			var looks int

			install(t, fakes{
				workloadD: func(string) (workload.Document, error) {
					looks++

					d := doc(t, liveWorkloadJSON)
					d["status"] = status

					if looks == 1 {
						d["status"] = workload.WorkloadStatusProvisioning
					}

					return d, nil
				},
				artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
				waitSteady: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
					return &workload.Workload{ID: id, Status: status}, nil
				},
			})

			bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

			_, stderr, err := runIn(t, bound, Options{NonInteractive: true})
			require.Error(t, err, "a broken workload is not a successful deploy")
			assert.NotContains(t, stderr, "Already up to date")
			assert.Contains(t, stderr, status, "the plan says what it found before the error explains it")
		})
	}
}

// TestRun_MissingWorkloadIsRecreated is the reported bug: a workload deleted
// out from under the manifest used to leave the user with a binding only a
// hand edit could clear. It is drift, so the deploy recreates and rebinds.
func TestRun_MissingWorkloadIsRecreated(t *testing.T) {
	var writtenID string

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(_, id string) error {
			writtenID = id

			return nil
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Equal(t, ActionCreated, result.Action)
	assert.Equal(t, "wl-new", writtenID, "the stale binding is replaced, not left for the user")
}

// TestRun_MissingWorkloadNamesTheDeadBinding: recreating is right when the
// workload was deleted and wrong when the run is pointed at an instance that
// never had it. `up` cannot tell those apart from a 404, and it applies
// without confirming, so naming the id it could not find is the only warning
// the wrong-instance case gets.
func TestRun_MissingWorkloadNamesTheDeadBinding(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	_, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "no longer exists")
	assert.Contains(t, stderr, "68b0c1d2", "the dead id is what tells a deletion from a wrong endpoint")
}

// TestRun_FailedRecreateReportsNoWorkloadID: a create that fails leaves
// nothing behind, and the id the file was bound to answers to nothing either.
// Reporting it in the envelope would hand a caller an id whose only use is a
// 404.
func TestRun_FailedRecreateReportsNoWorkloadID(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create: func(any) (*workload.Workload, error) {
			return nil, errors.New("platform said no")
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Empty(t, result.WorkloadID, "the dead binding is not this run's workload")
}

// TestRun_TerminatedWorkloadIsRefused: terminated looks like missing from the
// file's side and is not. The workload still exists, still holds its name and
// still owns its artifact, so a create would collide with it, and every line a
// create plan prints about it would be false. The message names the one thing
// that clears the way.
func TestRun_TerminatedWorkloadIsRefused(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return workload.Document{
				"id": "68b0c1d2e3f4a5b6c7d8e9f0", "name": "my-app",
				"status": workload.WorkloadStatusTerminated,
			}, nil
		},
		artifactD: func(string) (workload.Document, error) { return nil, nil },
		create: func(any) (*workload.Workload, error) {
			t.Fatal("a terminated workload still owns its name; creating over it collides")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	_, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "terminated")
	assert.Contains(t, err.Error(), "dr workload delete 68b0c1d2e3f4a5b6c7d8e9f0",
		"the remedy has to be one that works, and deleting it also clears the binding")
}

// A refusal must not have announced a create, because nothing is being
// created and the name it would print is the dead workload's.
func TestRun_TerminatedWorkloadDoesNotAnnounceACreate(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return workload.Document{
				"id": "68b0c1d2e3f4a5b6c7d8e9f0", "name": "old-name",
				"status": workload.WorkloadStatusTerminated,
			}, nil
		},
		artifactD: func(string) (workload.Document, error) { return nil, nil },
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	_, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)

	assert.NotContains(t, stderr, "will be created")
	assert.NotContains(t, stderr, "no longer exists", "it does still exist")
}

// track records what the build path did, in order, so a test can pin the
// sequence as well as the outcome. The order is the whole design: an artifact
// to hold the code, then the code, then an image, then a workload to run it.
type track struct {
	steps      []string
	artifactIn any
	linkedTo   wapi.InitOptions
	workloadIn any

	// savedCfg is the project state after a roll re-pointed it, and carried
	// is the (artifact, catalog, version) a spec-only roll inherited.
	savedCfg wapi.Config
	carried  []string

	// rolledRuntime is the sizing that travelled with the swap, nil when the
	// deploy changed only the version.
	rolledRuntime json.RawMessage
}

// wiredBuild is a build path where every step works, over the track that
// records it. Individual tests override one seam to break one step.
func wiredBuild(tr *track) fakes {
	return fakes{
		code: func(Loaded, Live) (CodeChange, error) {
			return CodeChange{Applies: true, FirstDeploy: true}, nil
		},
		newArtifact: func(payload any) (*workload.Artifact, error) {
			tr.steps = append(tr.steps, "create-artifact")
			tr.artifactIn = payload

			return &workload.Artifact{ID: "art-1", Status: workload.ArtifactStatusDraft}, nil
		},
		link: func(dir string, opts wapi.InitOptions) error {
			tr.steps = append(tr.steps, "link")
			tr.linkedTo = opts

			return nil
		},
		sync: func(string) (*sync.Result, error) {
			tr.steps = append(tr.steps, "sync")

			return &sync.Result{UploadedCount: 3, NewVersion: "ver-1"}, nil
		},
		build: func(string) (*workload.BuildTriggerResponse, error) {
			tr.steps = append(tr.steps, "build")

			return &workload.BuildTriggerResponse{BuildIDs: []string{"bld-1"}}, nil
		},
		waitBuild: func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
			return &workload.Build{ID: id, Status: workload.BuildStatusCompleted}, nil
		},
		create: func(payload any) (*workload.Workload, error) {
			tr.steps = append(tr.steps, "create-workload")
			tr.workloadIn = payload

			return running("wl-1"), nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
	}
}

// decodePayload reads back what was sent to a create endpoint.
func decodePayload(t *testing.T, payload any) map[string]any {
	t.Helper()

	raw, ok := payload.(json.RawMessage)
	require.True(t, ok, "the create clients are handed raw JSON")

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(raw, &decoded))

	return decoded
}

// TestRun_FirstDeployBuildsThenCreates is the shape of a deploy the platform
// builds: four acts in an order chosen so that a run which dies partway
// leaves something the next run can pick up, rather than a workload pointing
// at an image that does not exist.
func TestRun_FirstDeployBuildsThenCreates(t *testing.T) {
	var tr track

	install(t, wiredBuild(&tr))

	result, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"create-artifact", "link", "sync", "build", "create-workload"}, tr.steps)
	assert.Equal(t, "art-1", tr.linkedTo.ArtifactID)
	assert.Equal(t, "wl-1", result.WorkloadID)
	assert.Equal(t, "bld-1", result.BuildID, "the caller's envelope has to be able to name the build")
	assert.Contains(t, stderr, "Building the image")

	// The artifact is created from the manifest's own artifact block, which
	// is the create request already: nothing is translated on the way.
	assert.Equal(t, "my-app-artifact", decodePayload(t, tr.artifactIn)["name"])

	// The workload is then created against that artifact rather than with a
	// second copy of the block, which would ask for a second, empty artifact.
	sent := decodePayload(t, tr.workloadIn)
	assert.Equal(t, "art-1", sent["artifactId"])
	assert.NotContains(t, sent, "artifact")
	assert.Contains(t, sent, "runtime", "the sizing half still travels with the create")
}

// A project that is already linked keeps its artifact. Creating a second one
// would orphan the code that was pushed to the first.
func TestRun_LinkedProjectReusesItsArtifact(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-existing"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = linkedArtifactDoc
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("the project is already linked; a second artifact would orphan the first")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"sync", "build", "create-workload"}, tr.steps)
	assert.Equal(t, "art-existing", decodePayload(t, tr.workloadIn)["artifactId"])
}

// The platform allows one workload per draft artifact, so a stale link sends
// the deploy at an artifact that is already spoken for. Its own message names
// an id and nothing about where that id came from, which leaves the reader
// with a conflict and no idea what chose the thing that conflicted.
func TestRun_ConflictOnAReusedArtifactExplainsTheLink(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-existing"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = linkedArtifactDoc
	f.create = func(any) (*workload.Workload, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
	}
	f.list = func(int, []string, string) ([]workload.Workload, error) {
		return []workload.Workload{{ID: "wl-owner", Name: "someone-else", ArtifactID: "art-existing"}}, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "art-existing", "the artifact the link chose")
	assert.Contains(t, err.Error(), ".datarobot", "where that choice is recorded")
	assert.Contains(t, err.Error(), "wl-owner", "the workload that already has it")
	assert.Contains(t, err.Error(), "workloadId: wl-owner", "a line the reader can paste")
}

// A 409 on a linked project is just as likely to be a duplicate workload
// name, which create() has already explained accurately. Rewriting that as a
// link problem would send the user to delete the link and orphan an artifact
// for a reason that was never the cause.
func TestRun_NameConflictOnALinkedProjectKeepsItsOwnMessage(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-existing"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = linkedArtifactDoc
	f.create = func(any) (*workload.Workload, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
	}
	// Nothing holds the artifact; the name is what collided.
	f.list = func(int, []string, string) ([]workload.Workload, error) {
		return []workload.Workload{{ID: "wl-1", Name: "my-app", ArtifactID: "art-somewhere-else"}}, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "workloadId: wl-1", "the name conflict's own advice")
	assert.NotContains(t, err.Error(), "delete", "nothing here calls for deleting the link")
}

// The same conflict against an artifact this run just minted is not a stale
// link, so it must not be explained as one.
func TestRun_ConflictOnAFreshArtifactIsNotBlamedOnTheLink(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.create = func(any) (*workload.Workload, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
	}
	f.list = func(int, []string, string) ([]workload.Workload, error) { return nil, nil }

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "only one workload per draft artifact")
}

// lockedLink is a project pointed at an artifact that has since been locked,
// which is what a deploy that locked its own artifact leaves behind. The
// repository is the lineage the replacement has to join.
func lockedLink(f fakes, tr *track) fakes {
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-locked")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{
			ID: id, Status: workload.ArtifactStatusLocked, ArtifactRepositoryID: "repo-1",
		}, nil
	}
	// The same artifact as the server sends it. One read answers the lock, the
	// lineage and the spec, so the document is what the deploy actually reads.
	f.artifactD = func(id string) (workload.Document, error) {
		return workload.Document{
			"id":                   id,
			"status":               workload.ArtifactStatusLocked,
			"artifactRepositoryId": "repo-1",
			"type":                 "service",
		}, nil
	}
	f.save = relinkStep(tr)

	return f
}

// A locked artifact can take neither code nor an image, and locking cannot be
// undone, so the deploy makes the version the lock made necessary rather than
// refusing. The link moves with it, which is what keeps the catalog and the
// last-synced version: telling the reader to delete the state directory works
// too, and costs a full re-upload of a tree the platform already holds.
func TestRun_LockedLinkIsRedrafted(t *testing.T) {
	var tr track

	install(t, lockedLink(wiredBuild(&tr), &tr))

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"create-artifact", "relink", "sync", "build", "create-workload"}, tr.steps)
	assert.Equal(t, "art-1", tr.savedCfg.ArtifactID, "the project has to push at the version that can take code")
	require.NotNil(t, tr.savedCfg.CatalogID, "the code store has to survive the redraft")
	assert.Equal(t, "cat1", *tr.savedCfg.CatalogID,
		"the code store survives the redraft, or the next sync re-uploads the whole tree")

	// The replacement joins the lineage it replaces. Without the repository the
	// Registry grows a second entry under the same name on every lock, and the
	// workload's versions stop reading as versions of one thing.
	payload, ok := tr.artifactIn.(json.RawMessage)
	require.True(t, ok, "the create has to have been given the artifact block")
	assert.Contains(t, string(payload), "repo-1")

	// The whole point is which artifact the rest of the run targets. Step names
	// alone would stay identical if the build and the create kept using the
	// locked id, which is the one outcome this must never produce.
	workloadIn, ok := tr.workloadIn.(json.RawMessage)
	require.True(t, ok)
	assert.Contains(t, string(workloadIn), "art-1")
	assert.NotContains(t, string(workloadIn), "art-locked")
}

// The tree was already synced into the artifact that got locked, so the sync
// onto its replacement can have nothing to upload, and the new version would
// go to the builder with no code reference at all. The code the locked one
// was running is carried across instead.
func TestRun_RedraftedArtifactInheritsTheSyncedCode(t *testing.T) {
	var tr track

	f := lockedLink(wiredBuild(&tr), &tr)
	f.sync = emptySync(&tr)
	f.codeRef = carryCodeStep(&tr)

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"art-1", "cat1", "ver1"}, tr.carried)
	assert.Contains(t, tr.steps, "carry-code")
}

// A first deploy whose sync uploaded nothing has no code anywhere: not on the
// artifact, and not in a previous version to carry over. Building it would ask
// the platform to make an image out of nothing and fail minutes later saying
// so in its own words, so the run stops here and says which directory it
// looked in.
func TestRun_FirstDeployWithNothingToUploadStopsBeforeTheBuild(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.sync = emptySync(&tr)

	// What the link this run just wrote holds: an artifact and nothing else,
	// because no sync has ever produced a version to record.
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-1"}, nil }
	f.codeRef = func(string, string, string) error {
		t.Fatal("a first deploy has no earlier version whose code it could carry over")

		return nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to build")
	assert.NotContains(t, tr.steps, "build", "an artifact with no code must not reach the builder")
}

// The artifact exists and only the local record of it failed to write, which
// is the one failure that strands something. The error has to carry the id,
// or the next run creates a second artifact and the first is unreachable.
func TestRun_LinkFailureNamesTheStrandedArtifact(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.link = func(string, wapi.InitOptions) error { return errors.New("permission denied") }

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-1")
	assert.Contains(t, err.Error(), "dr artifact code init art-1")
	assert.NotContains(t, tr.steps, "sync")
}

// The other half of the same accident, on a directory that is already linked,
// which is what every roll and every redraft is. The advice has to differ:
// 'dr artifact code init' refuses a directory holding state, so naming it here
// would send the reader in a circle, and deleting the state to force it would
// throw away the catalog the redraft exists to keep. The artifact id still has
// to reach the envelope, or a run that stranded one reports none.
func TestRun_RelinkFailureNamesTheFileToEditInstead(t *testing.T) {
	var tr track

	f := lockedLink(wiredBuild(&tr), &tr)
	f.save = func(string, wapi.Config) error { return errors.New("permission denied") }

	install(t, f)

	result, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "art-1", "the stranded artifact has to be named")
	assert.Contains(t, err.Error(), "artifactId")
	assert.Contains(t, err.Error(), "config.json")
	assert.NotContains(t, err.Error(), "dr artifact code init",
		"that command refuses a directory that already has state")

	assert.Equal(t, "art-1", result.ArtifactID,
		"a run that created an artifact and then stranded it still has to report which")
	assert.NotContains(t, tr.steps, "sync", "nothing may be pushed once the link is unknown")
}

// A build that fails is the end of the deploy. Creating the workload anyway
// would produce something that cannot start, and the message points at the
// logs that say why rather than repeating that it failed.
func TestRun_FailedBuildStopsAndNamesTheLogs(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	// WaitForBuild's own contract: on a terminal error status it returns the
	// build and an error together. Stubbing a nil error here would test a
	// wait that does not exist and hide the id being dropped.
	f.waitBuild = func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: id, Status: workload.BuildStatusFailed},
			fmt.Errorf("build %s ended with status %s", id, workload.BuildStatusFailed)
	}

	install(t, f)

	result, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr artifact build logs art-1 bld-1")
	assert.NotContains(t, tr.steps, "create-workload")
	assert.Equal(t, "bld-1", result.BuildID, "a failed build is still the build to go and read")
}

// A wait that runs out returns the build it was still watching, and that id
// is the only way to follow the build to the end.
func TestRun_TimedOutBuildKeepsItsID(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.waitBuild = func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: id, Status: workload.BuildStatusInProgress},
			fmt.Errorf("timeout waiting for build %s", id)
	}

	install(t, f)

	result, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.NotContains(t, tr.steps, "create-workload")
	assert.Equal(t, "bld-1", result.BuildID)
}

// The build is waited for even under --detach. --detach promises not to wait
// for the workload to serve, not to hand back a workload that cannot start.
func TestRun_DetachStillWaitsForTheImage(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.wait = func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		t.Fatal("--detach does not wait for the workload")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)
	assert.Contains(t, tr.steps, "build")
}

// An artifact that has already built the code in the tree is not rebuilt.
// This is the second run after a first deploy that failed at the workload,
// and a rebuild there is minutes spent to produce the image already sitting
// on the artifact.
func TestRun_UnchangedTreeWithAnImageSkipsTheBuild(t *testing.T) {
	var tr track

	f := linkedAndSynced(&tr)
	f.builds = func(string, int) ([]workload.Build, error) {
		return []workload.Build{{ID: "bld-0", Status: workload.BuildStatusCompleted}}, nil
	}

	install(t, f)

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"sync", "create-workload"}, tr.steps)
	assert.Contains(t, stderr, "Image is up to date")
}

// The same skip, from a build the platform reported in lower case. hasImage
// asks the question BuildSummaryFor asks, so it folds the way that does: left
// exact, a completed build reads as no image at all and every deploy spends
// minutes rebuilding one that is already sitting on the artifact.
func TestRun_UnchangedTreeWithALowercaseCompletedBuildSkipsTheBuild(t *testing.T) {
	var tr track

	f := linkedAndSynced(&tr)
	f.builds = func(string, int) ([]workload.Build, error) {
		return []workload.Build{{ID: "bld-0", Status: "completed"}}, nil
	}

	install(t, f)

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"sync", "create-workload"}, tr.steps)
	assert.Contains(t, stderr, "Image is up to date")
}

// The same run where the previous attempt never got as far as an image. The
// question is asked of the platform rather than assumed, because assuming
// either way is wrong half the time.
func TestRun_UnchangedTreeWithNoImageStillBuilds(t *testing.T) {
	var tr track

	f := linkedAndSynced(&tr)
	f.builds = func(string, int) ([]workload.Build, error) {
		return []workload.Build{{ID: "bld-0", Status: workload.BuildStatusFailed}}, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"sync", "build", "create-workload"}, tr.steps)
}

// --force-build is the answer to code that reached the artifact by some route
// the deploy cannot see, which leaves a completed build of the wrong version
// and a tree that looks unchanged.
func TestRun_ForceBuildRebuildsAnUnchangedTree(t *testing.T) {
	var tr track

	f := linkedAndSynced(&tr)
	f.builds = func(string, int) ([]workload.Build, error) {
		t.Fatal("--force-build has already answered the question")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, ForceBuild: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"sync", "build", "create-workload"}, tr.steps)
}

// linkedAndSynced is a project that has deployed before and whose working
// tree has not moved since, which is the state where the build becomes a
// question rather than a certainty.
func linkedAndSynced(tr *track) fakes {
	f := wiredBuild(tr)

	f.code = func(Loaded, Live) (CodeChange, error) { return CodeChange{Applies: true}, nil }
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-1"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = linkedArtifactDoc
	f.sync = func(string) (*sync.Result, error) {
		tr.steps = append(tr.steps, "sync")

		return &sync.Result{NewVersion: "ver-1"}, nil
	}

	return f
}

// A conflict is not a failure, and it cannot be silent either: what was just
// uploaded is not what the user last looked at.
func TestRun_SyncConflictsAreReported(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.sync = func(string) (*sync.Result, error) {
		tr.steps = append(tr.steps, "sync")

		return &sync.Result{UploadedCount: 1, ConflictCount: 1, ConflictCopies: []string{"app.py.LOCAL.170"}}, nil
	}

	install(t, f)

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "app.py.LOCAL.170")
}

// Sizing the working tree is the whole of what a --dry-run does, and it is
// where the engine reads the ignore file, so a preview that stayed silent
// would be a preview of a different run than the one that follows.
func TestRun_DryRunReportsTheIgnoreFileNotice(t *testing.T) {
	const notice = "Using .wapiignore for sync ignore patterns."

	var tr track

	f := wiredBuild(&tr)
	f.code = func(Loaded, Live) (CodeChange, error) {
		return CodeChange{Applies: true, FirstDeploy: true, IgnoreNotice: notice}, nil
	}

	install(t, f)

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)

	assert.NotContains(t, tr.steps, "sync", "a dry run must not sync")
	assert.Equal(t, 1, strings.Count(stderr, notice))
}

// A flag that was asked for and did nothing has to say so, or it reads as a
// rebuild that quietly happened.
func TestRun_ForceBuildWithNothingToDoSaysSo(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, stderr, err := runIn(t, bound, Options{NonInteractive: true, ForceBuild: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "--force-build had no effect")
}

// The quieter half of the same problem: a manifest naming a published image
// deploys and succeeds, so a --force-build that was never going to build
// anything leaves nothing to suggest the image is not what the flag implied.
func TestRun_ForceBuildOnAPublishedImageSaysSo(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		build: func(string) (*workload.BuildTriggerResponse, error) {
			t.Fatal("a published image is not built by the platform")

			return nil, nil
		},
	})

	result, stderr, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, ForceBuild: true})
	require.NoError(t, err, "the deploy still goes ahead; only the flag was idle")

	assert.Equal(t, ActionCreated, result.Action)
	assert.Contains(t, stderr, "--force-build had no effect")
	assert.Contains(t, stderr, "does not build")
}

// The flag is not idle on a build track, so saying it was would be a lie in
// the one case it actually did something.
func TestRun_ForceBuildOnABuildTrackSaysNothing(t *testing.T) {
	var tr track

	install(t, linkedAndSynced(&tr))

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, ForceBuild: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "build", "the flag did something here")
	assert.NotContains(t, stderr, "--force-build had no effect")
}

// A file bound to an existing artifact has no build mode to read, and gating
// the create on the image mode refused it with a message about the platform
// building an image it was never asked to build.
func TestRun_CreatesFromAnArtifactTheManifestNames(t *testing.T) {
	var payload any

	install(t, fakes{
		create: func(p any) (*workload.Workload, error) {
			payload = p

			return running("wl-new"), nil
		},
		writeID: func(string, string) error { return nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
		},
	})

	result, _, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.NotNil(t, payload)
	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Equal(t, ActionCreated, result.Action)
	assert.False(t, result.Locked, "the artifact the file names is a draft")
}

// A create bound to an artifact that is already locked is a promotion: the
// version is permanent before this run touches anything. Locked comes off the
// artifact the workload is running, and a create has no workload, so without
// asking the file's artifact directly the result would call a permanent deploy
// a draft and the caller would advise locking what cannot be locked twice.
func TestRun_CreatesFromALockedArtifactReportsItLocked(t *testing.T) {
	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
	})

	result, _, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.True(t, result.Locked)
}

// The read is not optional. The id came out of the committed file, so an
// artifact that cannot be read is one the create was going to fail on anyway,
// and failing here costs a plan rather than a half-made workload.
func TestRun_CreateBoundToAnUnreadableArtifactFails(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			t.Fatal("nothing may be created once the artifact could not be read")

			return nil, nil
		},
		getArtifact: func(string) (*workload.Artifact, error) {
			return nil, errors.New("boom")
		},
	})

	result, _, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already locked")
	assert.Equal(t, ActionUnchanged, result.Action,
		"this fails before apply is reached, so the envelope must not report the create the plan wanted")
}

func TestRun_DryRunOnABuildTrackSucceeds(t *testing.T) {
	install(t, fakes{
		code: func(Loaded, Live) (CodeChange, error) {
			return CodeChange{Applies: true, FirstDeploy: true}, nil
		},
	})

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err, "planning works on every track even where applying does not")
	assert.Contains(t, stderr, "uploaded for the first time")
}

// draftArtifact is the live fixture before anyone locked it. The shared one is
// locked, which is right for the roll tests and wrong for anything asking what
// --lock does, since a locked artifact is exactly the case that short-circuits.
func draftArtifact(t *testing.T) workload.Document {
	t.Helper()

	d := doc(t, liveArtifactJSON)
	d["status"] = workload.ArtifactStatusDraft

	return d
}

// stoppedWorkload is the live fixture, off. Everything else about it still
// matches boundLiveManifest, so the only thing a plan can find is that it is
// not running.
func stoppedWorkload(t *testing.T) workload.Document {
	t.Helper()

	d := doc(t, liveWorkloadJSON)
	d["status"] = workload.WorkloadStatusStopped

	return d
}

// TestRun_StartsAStoppedWorkload: `up` means make the file true, and a
// workload the file describes exactly but which is switched off is not true
// yet. Nothing is created and nothing is rolled; it is one POST.
func TestRun_StartsAStoppedWorkload(t *testing.T) {
	var started string

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			started = id

			return &workload.WorkloadOperationResponse{WorkloadID: id, Status: "queued"}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
		create: func(any) (*workload.Workload, error) {
			t.Fatal("the workload exists; starting it is not creating a second one")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", started)
	assert.Equal(t, ActionStarted, result.Action)
	assert.Equal(t, workload.WorkloadStatusRunning, result.Status)
	assert.Contains(t, stderr, "Starting workload")
}

// The reported dead end: a stopped workload whose file asks for more than a
// start used to be refused, with a remedy naming two more commands. It is one
// run now, and the plan says both halves of it out loud.
func TestRun_StoppedWithDriftStartsAndThenReconciles(t *testing.T) {
	var order []string

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			order = append(order, "start")

			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		settings: func(id string, _ json.RawMessage) (*workload.Replacement, error) {
			order = append(order, "settings")

			return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
		},
		waitReplace: func(_ string, started *workload.Replacement, _, _ time.Duration,
			_ func(*workload.Replacement),
		) (*workload.Replacement, error) {
			started.Status = workload.ReplacementStatusCompleted

			return started, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			order = append(order, "wait")

			return running(id), nil
		},
	})

	// One resource figure differs, which makes the run a retune as well as a
	// start.
	drifted := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest, "cpu: 3", "cpu: 4", 1)

	result, stderr, err := runIn(t, drifted, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"start", "wait", "settings", "wait"}, order,
		"the workload has to be up before a settings change can land on it")
	assert.Equal(t, ActionUpdated, result.Action)
	assert.Contains(t, stderr, "started, having been stopped")
	assert.Contains(t, stderr, "runtime")
}

func TestRun_StartFailureNamesTheWorkload(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(string) (*workload.WorkloadOperationResponse, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			t.Fatal("there is nothing to wait for when the start was refused")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot start workload 68b0c1d2e3f4a5b6c7d8e9f0")
}

func TestRun_DetachedStartDoesNotWait(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			t.Fatal("--detach returns as soon as the start is requested")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)

	assert.Equal(t, ActionStarted, result.Action)
	assert.Contains(t, stderr, "start requested")
	assert.Equal(t, workload.WorkloadStatusSubmitted, result.Status,
		"reporting the status it was asked to leave reads as a run that did nothing")
}

// The platform no-ops a start of a suspended workload, so accepting one would
// poll for a transition that is never coming.
func TestRun_SuspendedIsRefusedRatherThanPolled(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			d := doc(t, liveWorkloadJSON)
			d["status"] = workload.WorkloadStatusSuspended

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "suspended")
}

// An interrupted workload is one the platform took down and will start again.
func TestRun_InterruptedStartsLikeAnyStoppedWorkload(t *testing.T) {
	var started string

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			d := doc(t, liveWorkloadJSON)
			d["status"] = workload.WorkloadStatusInterrupted

			return d, nil
		},
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			started = id

			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", started)
	assert.Equal(t, ActionStarted, result.Action)
}

// A start the platform ignored comes back 200 with a message saying so, and a
// green checkmark alone would be the whole account of it.
func TestRun_StartAcknowledgementIsPrinted(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{WorkloadID: id, Status: "Proton is already running"}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, stderr, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "already running")
}

// --lock on a workload already running a locked artifact has nothing to do,
// and locking twice is not a no-op at the platform.
func TestRun_StartDoesNotRelockALockedArtifact(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			return running(id), nil
		},
		lock: func(string) (*workload.Artifact, error) {
			t.Fatal("the artifact this workload runs is already locked")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)
	assert.True(t, result.Locked)
}

// unlockedArtifact is the live artifact as a draft. liveArtifactJSON is
// locked, which is the right shape for most of these tests and the wrong one
// for asking whether --lock still does anything.
func unlockedArtifact(t *testing.T) workload.Document {
	t.Helper()

	d := doc(t, liveArtifactJSON)
	d["status"] = workload.ArtifactStatusDraft

	return d
}

// The other half of --lock on the start path. The test above starts from an
// artifact that is already locked, so it only reaches the skip branch; this
// one has to actually lock, and not until the workload is serving.
func TestRun_StartLocksAnUnlockedArtifactOnceItServes(t *testing.T) {
	var order []string

	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return unlockedArtifact(t), nil },
		start: func(id string) (*workload.WorkloadOperationResponse, error) {
			order = append(order, "start")

			return &workload.WorkloadOperationResponse{WorkloadID: id}, nil
		},
		wait: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			order = append(order, "wait")

			return running(id), nil
		},
		lock: func(id string) (*workload.Artifact, error) {
			order = append(order, "lock:"+id)

			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true, Lock: true})
	require.NoError(t, err)

	assert.Equal(t, ActionStarted, result.Action, "a start, not a create: --lock has to work on this path too")
	assert.Equal(t, []string{"start", "wait", "lock:art-1"}, order,
		"locking is one-way, so it waits until the workload is actually serving")
	assert.True(t, result.Locked)
}

// The live pair for unboundCredentialManifest, stopped. It agrees with the
// file in every respect, so the only thing the plan can find is that the
// workload is off, which is what makes this a start rather than a roll.
const (
	stoppedCredWorkloadJSON = `{
  "id": "68b0c1d2e3f4a5b6c7d8e9f0",
  "name": "my-app",
  "status": "stopped",
  "importance": "low",
  "artifactId": "68a0000000000000000000a1",
  "runtime": {
    "containerGroups": [
      {
        "name": "default",
        "replicaCount": 1,
        "containers": [
          {"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}
        ]
      }
    ]
  }
}`

	credArtifactJSON = `{
  "id": "68a0000000000000000000a1",
  "name": "my-app-artifact",
  "status": "draft",
  "spec": {
    "type": "service",
    "containerGroups": [
      {
        "name": "default",
        "containers": [
          {
            "name": "primary", "primary": true, "port": 8080,
            "imageUri": "registry/team/app:v1",
            "environmentVars": [
              {"source": "dr-credential", "name": "OPENAI_API_KEY",
               "drCredentialId": "68b0cccc0000000000000003", "key": "apiToken"}
            ]
          }
        ]
      }
    ]
  }
}`
)

// A credential deleted while the workload was off would otherwise surface as
// a container that will not come up, minutes later, saying nothing about the
// manifest.
func TestRun_StartVerifiesCredentialsFirst(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, stoppedCredWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, credArtifactJSON), nil },
		cred: func(string) (*workload.Credential, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		start: func(string) (*workload.WorkloadOperationResponse, error) {
			t.Fatal("nothing may start before the credentials are known to resolve")

			return nil, nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundCredentialManifest

	result, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Equal(t, ActionStarted, result.Plan.Action(),
		"the fixtures have to agree, or this is testing the roll path instead")
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

// TestRun_BadCredentialStopsBeforeTheCreate is the whole point of the check:
// the reference is syntactically fine, so nothing local can catch it, and
// without the lookup the mistake would surface as a container that will not
// start once the deploy has already been paid for.
func TestRun_BadCredentialStopsBeforeTheCreate(t *testing.T) {
	install(t, fakes{
		cred: func(string) (*workload.Credential, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create: func(any) (*workload.Workload, error) {
			t.Fatal("nothing may be created before the credentials are known to resolve")

			return nil, nil
		},
	})

	_, stderr, err := runIn(t, unboundCredentialManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, stderr, "+ workload", "the plan is printed before the refusal, as on any other track")
}

// A dry run reaches no mutation, so it never spends the lookups. Verifying
// would make --dry-run fail on a file it was asked only to describe.
func TestRun_DryRunDoesNotVerifyCredentials(t *testing.T) {
	install(t, fakes{
		cred: func(string) (*workload.Credential, error) {
			t.Fatal("a dry run mutates nothing, so it has nothing to verify against")

			return nil, nil
		},
	})

	_, _, err := runIn(t, unboundCredentialManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)
}

func TestRun_ResolvedCredentialDeploys(t *testing.T) {
	install(t, fakes{
		cred: func(id string) (*workload.Credential, error) {
			return &workload.Credential{CredentialID: id, Name: "my-app/OPENAI_API_KEY"}, nil
		},
		create: func(any) (*workload.Workload, error) { return running("wl-1"), nil },
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-1"), nil
		},
	})

	result, _, err := runIn(t, unboundCredentialManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Equal(t, "wl-1", result.WorkloadID)
}

// The first deploy of a project that was cloned rather than linked here is the
// run most likely to be someone's first sight of the old filename, and it is
// also the one run that builds no sizing engine: defaultCodeChange returns
// before that on an unlinked project. Reading the ignore file directly is what
// keeps the notice from waiting for the second deploy.
func TestDefaultCodeChange_ReportsLegacyIgnoreFileOnAnUnlinkedProject(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, unboundDockerfileManifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.LegacyFileName), []byte("*.tmp\n"), 0o644))

	loaded, err := load(dir, Options{NonInteractive: true})
	require.NoError(t, err)
	require.False(t, wapi.Exists(dir), "the point of this case is that nothing is linked yet")

	code, err := defaultCodeChange(loaded, Live{})
	require.NoError(t, err)

	assert.True(t, code.FirstDeploy)
	assert.Contains(t, code.IgnoreNotice, ignore.LegacyFileName)
	assert.Contains(t, code.IgnoreNotice, ignore.FileName, "and what to rename it to")
}

// The same project on the current name has nothing to report, which is what
// separates "the file was read" from "the notice is hardcoded on this path".
func TestDefaultCodeChange_SaysNothingForTheCurrentIgnoreFileName(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, unboundDockerfileManifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName), []byte("*.tmp\n"), 0o644))

	loaded, err := load(dir, Options{NonInteractive: true})
	require.NoError(t, err)

	code, err := defaultCodeChange(loaded, Live{})
	require.NoError(t, err)

	assert.True(t, code.FirstDeploy)
	assert.Empty(t, code.IgnoreNotice)
}

// TestRun_LinkedArtifactThatNoLongerMatchesIsReplaced is the recovery flow the
// missing-binding recreate advertises. The link records which artifact this
// checkout pushes to, not what that artifact holds, and an artifact's spec is
// fixed at creation. Reusing one whose spec has since diverged deploys the
// frozen answer and reports success, so the edit the user just made never
// leaves the machine.
func TestRun_LinkedArtifactThatNoLongerMatchesIsReplaced(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-stale"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.save = func(_ string, cfg wapi.Config) error {
		tr.linkedTo = wapi.InitOptions{ArtifactID: cfg.ArtifactID}

		return nil
	}
	// The live artifact says port 9999; the manifest says 8080.
	f.artifactD = func(string) (workload.Document, error) {
		return workload.Document{"spec": map[string]any{
			"type": "service",
			"containerGroups": []any{map[string]any{
				"name": "default",
				"containers": []any{map[string]any{
					"name": "primary", "primary": true, "port": 9999,
				}},
			}},
		}}, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact",
		"a linked artifact that no longer says what the file says has to be replaced, not reused")
	assert.NotEqual(t, "art-stale", decodePayload(t, tr.workloadIn)["artifactId"])
	assert.Equal(t, decodePayload(t, tr.workloadIn)["artifactId"], tr.linkedTo.ArtifactID,
		"the project has to end up pointing at the artifact it just deployed")
}

// divergedLink is a project whose linked artifact still takes code but no
// longer says what the file says: it holds port 9999 where the manifest asks
// for 8080. The repository is the lineage the replacement has to join, exactly
// as it is for a locked one.
func divergedLink(f fakes, tr *track) fakes {
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-stale")
	f.artifactD = func(id string) (workload.Document, error) {
		return workload.Document{
			"id":                   id,
			"status":               workload.ArtifactStatusDraft,
			"artifactRepositoryId": "repo-1",
			"type":                 "service",
			"spec": map[string]any{
				"containerGroups": []any{map[string]any{
					"name": "default",
					"containers": []any{map[string]any{
						"name": "primary", "primary": true, "port": 9999,
					}},
				}},
			},
		}, nil
	}
	f.save = relinkStep(tr)

	return f
}

// A spec that has diverged and a lock that cannot be undone are two reasons for
// the same act, so the replacement they produce has to be the same one. Locking
// got the lineage right and this route reached the create by a different path,
// which is where the two could quietly come apart: a replacement that opens a
// repository of its own leaves the Registry with a second entry under the same
// name, and the workload's versions stop reading as versions of one thing.
func TestRun_ReplacementForADivergedSpecJoinsTheSameLineage(t *testing.T) {
	var tr track

	install(t, divergedLink(wiredBuild(&tr), &tr))

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	payload, ok := tr.artifactIn.(json.RawMessage)
	require.True(t, ok, "the create has to have been given the artifact block")
	assert.Contains(t, string(payload), "repo-1")

	assert.Equal(t, "art-1", tr.savedCfg.ArtifactID, "the link moves to what the run just made")
	require.NotNil(t, tr.savedCfg.CatalogID, "the code store has to survive the replacement")
	assert.Equal(t, "cat1", *tr.savedCfg.CatalogID)
}

// The same lineage question asked of an agent, which is what makes it a
// regression test rather than a duplicate.
//
// The comparison that decides whether the linked draft still matches used to
// normalise the live document in place, taking the type off it. The next line
// read that type to pick the repository, found nothing, defaulted it to
// service, and concluded the file had changed the artifact's kind. Every
// redraft of an agent therefore started a new lineage instead of joining its
// own, silently and permanently. Nothing caught it because every other fixture
// here is a service, where the default happens to be the right answer.
func TestRun_ReplacementForADivergedAgentJoinsTheSameLineage(t *testing.T) {
	var tr track

	f := divergedLink(wiredBuild(&tr), &tr)

	inner := f.artifactD
	f.artifactD = func(id string) (workload.Document, error) {
		d, err := inner(id)
		if err != nil {
			return nil, err
		}

		d[keyType] = "agent"

		return d, nil
	}

	install(t, f)

	agent := strings.Replace(unboundDockerfileManifest, "type: service", "type: agent", 1)

	_, _, err := runIn(t, agent, Options{NonInteractive: true})
	require.NoError(t, err)

	payload, ok := tr.artifactIn.(json.RawMessage)
	require.True(t, ok, "the create has to have been given the artifact block")
	assert.Contains(t, string(payload), "repo-1",
		"an agent redraft joins its own lineage, exactly as a service does")
}

// The other half of that equivalence. The tree was already synced into the
// artifact the file has since diverged from, so the sync onto its replacement
// can find nothing to upload and the new version would go to the builder with
// no code reference at all. The code the old one was running is carried across
// instead, which is what makes this a new version of the same code rather than
// a build of nothing.
func TestRun_ReplacementForADivergedSpecInheritsTheSyncedCode(t *testing.T) {
	var tr track

	f := divergedLink(wiredBuild(&tr), &tr)
	f.sync = emptySync(&tr)
	f.codeRef = carryCodeStep(&tr)

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{"art-1", "cat1", "ver1"}, tr.carried)
	assert.Contains(t, tr.steps, "carry-code")
}

// A read that fails is not a difference. Minting on it would orphan the
// artifact this project pushes to because of a timeout, so the run stops and
// names the artifact it could not read instead of guessing either way.
//
// The lock check and the spec comparison are one read, so there is one failure
// to answer rather than two, and reusing an artifact whose state was never
// established is no better than replacing it: the sync that follows fetches the
// same artifact and would fail a step later with a worse message.
func TestRun_UnreadableLinkedArtifactStopsRatherThanGuessing(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-existing"}, nil }
	f.artifactD = func(string) (workload.Document, error) { return nil, errors.New("gateway timeout") }
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("a timeout is not a reason to start a new artifact lineage")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "art-existing", "the artifact the link names")
	assert.Contains(t, err.Error(), "linked to")
	assert.Contains(t, err.Error(), "gateway timeout", "the platform's own reason has to survive")
	assert.Nil(t, tr.workloadIn, "nothing is deployed on a state that was never established")
}

// StateMissing is the one state this change converted from a refusal into a
// mutating create, so --dry-run over it is the combination that most needs
// holding: a dry run that creates would be the worst possible regression here.
func TestRun_DryRunOnAMissingBindingMutatesNothing(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
		create: func(any) (*workload.Workload, error) {
			t.Fatal("--dry-run must not create a workload")

			return nil, nil
		},
		writeID: func(string, string) error {
			t.Fatal("--dry-run must not rebind the manifest")

			return nil
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	result, stderr, err := runIn(t, bound, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "no longer exists")
	assert.Equal(t, ActionCreated, result.Action, "the plan still says what it would do")
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", result.Plan.PriorWorkloadID)
}

// A create that reports no id cannot be linked to. SaveConfig does not
// validate the way Initialize does, so relinking to an empty id wrote a
// config.json every later command rejected as corrupt: the run failed anyway,
// but two steps later and having damaged the project on the way.
func TestRun_ArtifactCreatedWithoutAnIDIsRefusedBeforeLinking(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-stale"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = func(string) (workload.Document, error) {
		return workload.Document{"spec": map[string]any{"type": "service"}}, nil
	}
	f.newArtifact = func(any) (*workload.Artifact, error) { return &workload.Artifact{ID: ""}, nil }
	f.save = func(string, wapi.Config) error {
		t.Fatal("nothing may be written when there is no id to write")

		return nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reported no id")
}
