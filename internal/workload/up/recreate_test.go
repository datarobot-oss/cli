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
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundDeadManifest is a project bound to a workload that is past saving,
// which is the only state --recreate acts on.
const boundWorkloadID = "68b0c1d2e3f4a5b6c7d8e9f0"

func deadWorkload(status string) func(string) (workload.Document, error) {
	return func(string) (workload.Document, error) {
		return workload.Document{"id": boundWorkloadID, "name": "my-app", "status": status}, nil
	}
}

func boundManifestFor() string {
	return "workloadId: " + boundWorkloadID + "\n" + unboundImageManifest
}

// The whole point of the flag: the run that used to refuse now performs the
// exit its refusal names, and comes out the far side having created the
// workload again under the same name.
func TestRecreate_DeletesTheDeadWorkloadAndCreatesItAgain(t *testing.T) {
	for _, status := range []string{workload.WorkloadStatusErrored, workload.WorkloadStatusTerminated} {
		t.Run(status, func(t *testing.T) {
			var (
				deleted   string
				clearedID string
			)

			install(t, fakes{
				workloadD: deadWorkload(status),
				artifactD: func(string) (workload.Document, error) { return nil, nil },
				deleteWorkload: func(id string) error {
					deleted = id

					return nil
				},
				clearID: func(_, id string) (bool, error) {
					clearedID = id

					return true, nil
				},
				create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
				wait: func(string, workload.Serving, time.Duration, time.Duration,
					func(*workload.Workload),
				) (*workload.Workload, error) {
					return running("wl-new"), nil
				},
			})

			result, stderr, err := runIn(t, boundManifestFor(),
				Options{NonInteractive: true, Yes: true, Recreate: true})
			require.NoError(t, err)

			assert.Equal(t, boundWorkloadID, deleted, "the dead workload is what gets deleted")
			assert.Equal(t, boundWorkloadID, clearedID, "and the binding to it goes with it")
			assert.Equal(t, "wl-new", result.WorkloadID, "the run continues into the create")
			assert.Equal(t, "my-app", result.Name, "recreated under the name the file gives")
			assert.Contains(t, stderr, "Deleting the "+status+" workload")
		})
	}
}

// --recreate is not a way to replace a workload a deploy can act on. Widening
// it to a running one would make it "destroy production and rebuild it", which
// is not what it says.
func TestRecreate_RefusesAWorkloadADeployCanActOn(t *testing.T) {
	install(t, fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return nil, nil },
	})

	_, _, err := runIn(t, boundManifestFor(), Options{NonInteractive: true, Yes: true, Recreate: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--recreate")
	assert.Contains(t, err.Error(), "errored or terminated", "say what it does act on")
}

// A first run has nothing to delete, and failing it would refuse a run that was
// already about to do exactly what was asked.
func TestRecreate_NoOpsWhenThereIsNothingBound(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	result, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, Yes: true, Recreate: true})
	require.NoError(t, err)
	assert.Equal(t, "wl-new", result.WorkloadID)
}

// The one ordering hazard: the delete happens before the plan is printed, so a
// dry run that deleted would be a dry run that mutated. The unwired delete seam
// fails the test if this regresses.
func TestRecreate_DryRunDeletesNothing(t *testing.T) {
	install(t, fakes{
		workloadD: deadWorkload(workload.WorkloadStatusErrored),
		artifactD: func(string) (workload.Document, error) { return nil, nil },
	})

	_, stderr, err := runIn(t, boundManifestFor(),
		Options{NonInteractive: true, Yes: true, Recreate: true, DryRun: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "would delete workload "+boundWorkloadID)
}

// A dry run has to answer "what would this deploy", so it has to plan against
// the state the real run reaches rather than the one it starts from.
//
// Skipping the delete but keeping the dead workload described a deploy that
// cannot happen: the plan rendered a roll onto a workload the very next check
// refuses, and the envelope reported "rolled" for a run whose real action is
// "created" — while the hint above it said a create would happen.
func TestRecreate_DryRunPlansTheRunItWouldPerform(t *testing.T) {
	install(t, fakes{
		workloadD: deadWorkload(workload.WorkloadStatusErrored),
		artifactD: func(string) (workload.Document, error) { return nil, nil },
	})

	previewed, stderr, err := runIn(t, boundManifestFor(),
		Options{NonInteractive: true, Yes: true, Recreate: true, DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, ActionCreated, previewed.Plan.Action(),
		"the dead workload is deleted, so what follows is a create")
	assert.Contains(t, stderr, "will be created")
	assert.NotContains(t, stderr, "nothing healthy to deploy onto",
		"the plan must not describe a deploy onto the workload it just said would be deleted")

	// The same run for real, which is the thing the preview claims to predict.
	install(t, fakes{
		workloadD:      deadWorkload(workload.WorkloadStatusErrored),
		artifactD:      func(string) (workload.Document, error) { return nil, nil },
		deleteWorkload: func(string) error { return nil },
		clearID:        func(string, string) (bool, error) { return true, nil },
		create:         func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	performed, _, err := runIn(t, boundManifestFor(), Options{NonInteractive: true, Yes: true, Recreate: true})
	require.NoError(t, err)

	assert.Equal(t, performed.Action, previewed.Plan.Action(),
		"a preview that names a different action than the run is not a preview")
}

// Deleting is irreversible, so a run with nobody to ask and no --yes must stop
// rather than assume consent.
//
// The piped case is the one the command actually produces: `up` passes no
// Confirm when stdin is not a terminal and sets NonInteractive at the same
// time, so keying consent on NonInteractive made `--recreate < /dev/null`
// delete without a word and put this refusal out of reach. The unwired delete
// seam is what fails the test if that comes back.
func TestRecreate_WithoutATerminalOrYesRefuses(t *testing.T) {
	for name, opts := range map[string]Options{
		"no terminal":     {Recreate: true},
		"stdin is a pipe": {Recreate: true, NonInteractive: true},
	} {
		t.Run(name, func(t *testing.T) {
			install(t, fakes{
				workloadD: deadWorkload(workload.WorkloadStatusErrored),
				artifactD: func(string) (workload.Document, error) { return nil, nil },
			})

			_, _, err := runIn(t, boundManifestFor(), opts)
			require.Error(t, err)

			assert.Contains(t, err.Error(), "--yes", "name the way to say so explicitly")
			assert.Contains(t, err.Error(), "cannot be undone")
		})
	}
}

// The safety story of the flag is that the workload's name has to be typed
// back, and a prompt that accepts anything is not that. What the deploy hands
// Confirm as the expected answer is the assertion: nothing else in the run
// checks it, so a literal there would go unnoticed.
func TestRecreate_AsksForTheWorkloadNameToBeTypedBack(t *testing.T) {
	var (
		asked  string
		expect string
	)

	install(t, fakes{
		workloadD:      deadWorkload(workload.WorkloadStatusErrored),
		artifactD:      func(string) (workload.Document, error) { return nil, nil },
		deleteWorkload: func(string) error { return nil },
		clearID:        func(string, string) (bool, error) { return true, nil },
		create:         func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	result, _, err := runIn(t, boundManifestFor(), Options{
		Recreate: true,
		Confirm: func(question, want string) (bool, error) {
			asked, expect = question, want

			return true, nil
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "my-app", expect, "the workload name is what has to be typed")
	assert.Contains(t, asked, "my-app", "the question names the workload being deleted")
	assert.Contains(t, asked, "cannot be undone")
	assert.Equal(t, "wl-new", result.WorkloadID, "agreeing carries the run into the create")
}

// Declining leaves the workload alone and the run does not proceed to create a
// second one beside it.
func TestRecreate_DecliningStopsTheRun(t *testing.T) {
	install(t, fakes{
		workloadD: deadWorkload(workload.WorkloadStatusErrored),
		artifactD: func(string) (workload.Document, error) { return nil, nil },
	})

	_, _, err := runIn(t, boundManifestFor(), Options{
		Recreate: true,
		Confirm:  func(string, string) (bool, error) { return false, nil },
	})

	// The run stops at the refusal the dead state already carries, having
	// deleted nothing: the unwired delete seam would have failed the test.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is errored")
}

// The delete has already happened by the time the binding is cleared, so a
// failure there must not stop a run whose irreversible half is done.
func TestRecreate_AFailedBindingClearDoesNotStopTheRun(t *testing.T) {
	install(t, fakes{
		workloadD:      deadWorkload(workload.WorkloadStatusErrored),
		artifactD:      func(string) (workload.Document, error) { return nil, nil },
		deleteWorkload: func(string) error { return nil },
		clearID: func(string, string) (bool, error) {
			return false, errors.New("permission denied")
		},
		create: func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	result, stderr, err := runIn(t, boundManifestFor(), Options{NonInteractive: true, Yes: true, Recreate: true})
	require.NoError(t, err)

	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Contains(t, stderr, "permission denied", "say what could not be written")
}

// A delete that fails leaves the name still taken, so the run must stop rather
// than walk into the 409 it would cause.
func TestRecreate_AFailedDeleteStopsTheRun(t *testing.T) {
	install(t, fakes{
		workloadD:      deadWorkload(workload.WorkloadStatusErrored),
		artifactD:      func(string) (workload.Document, error) { return nil, nil },
		deleteWorkload: func(string) error { return errors.New("boom") },
	})

	_, _, err := runIn(t, boundManifestFor(), Options{NonInteractive: true, Yes: true, Recreate: true})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "still holds its name")
}
