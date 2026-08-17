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
	wizard    func(wizard.Options) (wizard.Result, error)
	create    func(any) (*workload.Workload, error)
	wait      func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
	list      func(int, []string) ([]workload.Workload, error)
	start     func(string) (*workload.WorkloadOperationResponse, error)
	lock      func(string) (*workload.Artifact, error)
	cred      func(string) (*workload.Credential, error)
	writeID   func(string, string) error
	code      func(Loaded, Live) (CodeChange, error)
	workloadD func(string) (workload.Document, error)
	artifactD func(string) (workload.Document, error)

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

	// swap leaves a seam alone when the test supplied nothing, so without this
	// a stopped fixture with no start fake POSTs to whatever tenant the
	// developer is logged into.
	force(t, &startWorkloadFn, func(id string) (*workload.WorkloadOperationResponse, error) {
		t.Fatalf("the run started workload %s, which this test did not wire", id)

		return nil, nil
	})

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
		list: func(int, []string) ([]workload.Workload, error) {
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
		list: func(int, []string) ([]workload.Workload, error) { return nil, errors.New("list unavailable") },
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
func TestRun_RefusesTheStatesThatCannotTakeADeploy(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{workload.WorkloadStatusTerminated, "remove workloadId"},
		{workload.WorkloadStatusErrored, "dr workload logs"},
		{workload.WorkloadStatusProvisioning, "still settling"},
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

func TestRun_MissingWorkloadNamesTheRebind(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
	})

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + unboundImageManifest

	_, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--workload-id")
	assert.Contains(t, err.Error(), "remove workloadId")
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

// A locked artifact can take neither code nor an image, so the deploy stops
// before the sync rather than failing halfway through with the platform's
// 403, which says nothing about what to do next.
func TestRun_LockedArtifactStopsBeforeThePush(t *testing.T) {
	var tr track

	f := wiredBuild(&tr)
	f.linked = func(string) bool { return true }
	f.project = func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-locked"}, nil }
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
	}

	install(t, f)

	_, _, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-locked")
	assert.Contains(t, err.Error(), "locked")
	assert.Empty(t, tr.steps, "nothing may be pushed at an artifact that cannot accept it")
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
	})

	result, _, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.NotNil(t, payload)
	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Equal(t, ActionCreated, result.Action)
}

func TestRun_DryRunOnABuildTrackSucceeds(t *testing.T) {
	install(t, fakes{
		code: func(Loaded, Live) (CodeChange, error) {
			return CodeChange{Applies: true, FirstDeploy: true}, nil
		},
	})

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err, "planning works on every track even where applying does not")
	assert.Contains(t, stderr, "first sync of this project")
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

// A plan that asks for more than a start is refused whole. Starting the
// workload first would bring it up on the version it was stopped on and then
// report a failure, which leaves the user worse off than refusing did.
func TestRun_StoppedWithDriftIsRefusedWithoutStarting(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return stoppedWorkload(t), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		start: func(string) (*workload.WorkloadOperationResponse, error) {
			t.Fatal("a refused plan must not have started anything")

			return nil, nil
		},
	})

	// One resource figure differs, which makes the run a retune as well as a
	// start.
	drifted := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" +
		strings.Replace(boundLiveManifest, "cpu: 3", "cpu: 4", 1)

	_, _, err := runIn(t, drifted, Options{NonInteractive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr workload start")
	assert.Contains(t, err.Error(), "more than starting it")
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
