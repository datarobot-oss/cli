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
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
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

// fakes collects the seams a run touches. A zero value is a run where every
// call fails loudly, so a test only wires what it means to exercise.
type fakes struct {
	wizard    func(wizard.Options) (wizard.Result, error)
	create    func(any) (*workload.Workload, error)
	wait      func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
	list      func(int, []string) ([]workload.Workload, error)
	lock      func(string) (*workload.Artifact, error)
	writeID   func(string, string) error
	code      func(Loaded, Live) (CodeChange, error)
	workloadD func(string) (workload.Document, error)
	artifactD func(string) (workload.Document, error)
}

// install swaps every seam and restores them afterwards.
func install(t *testing.T, f fakes) {
	t.Helper()

	prev := struct {
		wizard    func(wizard.Options) (wizard.Result, error)
		create    func(any) (*workload.Workload, error)
		wait      func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
		list      func(int, []string) ([]workload.Workload, error)
		lock      func(string) (*workload.Artifact, error)
		writeID   func(string, string) error
		code      func(Loaded, Live) (CodeChange, error)
		workloadD func(string) (workload.Document, error)
		artifactD func(string) (workload.Document, error)
	}{
		runWizardFn, createWorkloadFn, waitWorkloadFn, listWorkloadsFn,
		lockArtifactFn, writeWorkloadIDFn, codeChangeFn, getWorkloadDocFn, getArtifactDocFn,
	}

	if f.wizard != nil {
		runWizardFn = f.wizard
	}

	if f.create != nil {
		createWorkloadFn = f.create
	}

	if f.wait != nil {
		waitWorkloadFn = f.wait
	}

	if f.list != nil {
		listWorkloadsFn = f.list
	}

	if f.lock != nil {
		lockArtifactFn = f.lock
	}

	writeWorkloadIDFn = func(path, id string) error { return nil }
	if f.writeID != nil {
		writeWorkloadIDFn = f.writeID
	}

	codeChangeFn = func(Loaded, Live) (CodeChange, error) { return CodeChange{}, nil }
	if f.code != nil {
		codeChangeFn = f.code
	}

	if f.workloadD != nil {
		getWorkloadDocFn = f.workloadD
	}

	if f.artifactD != nil {
		getArtifactDocFn = f.artifactD
	}

	t.Cleanup(func() {
		runWizardFn, createWorkloadFn, waitWorkloadFn, listWorkloadsFn = prev.wizard, prev.create, prev.wait, prev.list
		lockArtifactFn, writeWorkloadIDFn, codeChangeFn = prev.lock, prev.writeID, prev.code
		getWorkloadDocFn, getArtifactDocFn = prev.workloadD, prev.artifactD
	})
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

// TestRun_ConflictAdoptsTheExistingWorkload: failing here would leave the
// user with a name they cannot deploy and no pointer to what owns it.
func TestRun_ConflictAdoptsTheExistingWorkload(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		list: func(int, []string) ([]workload.Workload, error) {
			return []workload.Workload{{ID: "wl-other", Name: "someone-else"}, *running("wl-existing")}, nil
		},
		wait: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-existing"), nil
		},
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Equal(t, "wl-existing", result.WorkloadID)
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
		{workload.WorkloadStatusStopped, "starting a stopped workload"},
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

// TestRun_BuildTracksPlanButDoNotApplyYet: the plan is correct and printed,
// and the refusal says which half is missing rather than failing obscurely.
func TestRun_BuildTracksPlanButDoNotApplyYet(t *testing.T) {
	install(t, fakes{
		code: func(Loaded, Live) (CodeChange, error) {
			return CodeChange{Applies: true, FirstDeploy: true}, nil
		},
		create: func(any) (*workload.Workload, error) {
			t.Fatal("the build tracks are not wired")

			return nil, nil
		},
	})

	_, stderr, err := runIn(t, unboundDockerfileManifest, Options{NonInteractive: true})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotWired)
	assert.Contains(t, err.Error(), "dockerfile")
	assert.Contains(t, stderr, "+ workload", "the plan is still printed before the refusal")
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

// The refusal has to name the change the plan asks for. Calling a
// runtime-only plan a roll contradicts the plan printed right above it.
func TestRun_RuntimeOnlyRefusalDoesNotClaimARoll(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
		create: func(any) (*workload.Workload, error) {
			t.Fatal("a live workload is not created again")

			return nil, nil
		},
	})

	// Same artifact as the live one, one runtime number moved: no new version
	// is minted, so nothing here is a roll.
	retuned := strings.Replace(boundLiveManifest, "cpu: 3", "cpu: 6", 1)
	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + retuned

	_, _, err := runIn(t, bound, Options{NonInteractive: true})
	require.ErrorIs(t, err, ErrNotWired)
	assert.Contains(t, err.Error(), "runtime settings")
	assert.NotContains(t, err.Error(), "new version", "nothing here mints an artifact")
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
