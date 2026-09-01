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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runConfirmIn is runIn with the stderr buffer supplied by the test, so the
// gate's recorder can measure what had been printed by the time the question
// was asked. Placement is most of what the gate has to get right, and offsets
// are how a buffer answers questions about order.
func runConfirmIn(t *testing.T, content string, opts Options, stderr *bytes.Buffer) (Result, error) {
	t.Helper()

	dir := t.TempDir()
	writeManifest(t, dir, content)

	// The validator refuses a provided-source build with no Dockerfile beside
	// the manifest, which is correct and means the build-track fixtures need
	// a real one. Writing it unconditionally is harmless for the others.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM scratch\nEXPOSE 8080\n"), 0o600))

	opts.Dir = dir
	opts.Stderr = stderr

	return Run(opts)
}

// liveDocs is the bound pair the empty-plan and retune fixtures deploy
// against, and the decline tests wire counters on top of it.
func liveDocs(t *testing.T) fakes {
	return fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
	}
}

// TestRun_ConfirmPromptsAfterPlan is the go-test half of the prompt contract:
// the gate asks once, after the plan has been rendered, and the question is
// the one the y/N answer is graded against. The decline also pins the run's
// own half of the exit contract: ErrDeclined, from a run that has printed the
// plan and mutated nothing.
func TestRun_ConfirmPromptsAfterPlan(t *testing.T) {
	install(t, liveDocs(t))

	var (
		asked    int
		question string
		printed  int
	)

	var stderr bytes.Buffer

	result, err := runConfirmIn(t, resizedManifest(), Options{
		ConfirmApply: func(q string) (bool, error) {
			asked++
			question = q
			printed = stderr.Len()

			return false, nil
		},
	}, &stderr)

	require.ErrorIs(t, err, ErrDeclined)
	assert.Equal(t, 1, asked, "the gate is asked once, not once per branch")
	assert.Equal(t, "Apply this deploy?", question)
	assert.Contains(t, stderr.String()[:printed], "-> ",
		"the default plan precedes the question: consent follows the review")
	assert.Equal(t, ActionUnchanged, result.Action, "a decline did nothing, so nothing is reported")
}

// TestRun_ConfirmGate_AfterDryRunReturn: a preview is not a mutation to
// consent to, so the dry-run return comes first and the gate is never armed.
func TestRun_ConfirmGate_AfterDryRunReturn(t *testing.T) {
	f := liveDocs(t)
	f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("a dry run must not touch the workload")

		return nil, nil
	}

	install(t, f)

	var asked bool

	result, stderr, err := runIn(t, resizedManifest(), Options{
		NonInteractive: true,
		DryRun:         true,
		ConfirmApply:   func(string) (bool, error) { asked = true; return false, nil },
	})

	require.NoError(t, err)
	assert.False(t, asked, "dry-run never prompts")
	assert.Equal(t, ActionUpdated, result.Action, "the dry run's action is still the plan's own")
	assert.Contains(t, stderr, "-> ", "the plan is printed as usual")
}

// TestRun_ConfirmGate_AfterNoteUnusedForce: the --force-build note is part of
// what the user reviews, so it is said before the question, not after it.
func TestRun_ConfirmGate_AfterNoteUnusedForce(t *testing.T) {
	install(t, liveDocs(t))

	var printed int

	var stderr bytes.Buffer

	_, err := runConfirmIn(t, resizedManifest(), Options{
		ForceBuild:   true,
		ConfirmApply: func(string) (bool, error) { printed = stderr.Len(); return false, nil },
	}, &stderr)

	require.ErrorIs(t, err, ErrDeclined)

	note := strings.Index(stderr.String(), "--force-build had no effect")
	require.NotEqual(t, -1, note, "the unused-force note is part of the output")
	assert.Less(t, note, printed, "the note precedes the question")
}

// TestRun_ConfirmAccept_Proceeds: yes means the run continues through the
// normal apply path exactly as if the gate had never been installed. The seam
// sequence is captured twice -- once without the gate, once accepting it --
// and the two must agree, because --confirm is a question, not a new plan.
func TestRun_ConfirmAccept_Proceeds(t *testing.T) {
	seams := func(seen *[]string) fakes {
		f := liveDocs(t)
		f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
			*seen = append(*seen, "resize")

			return nil, nil
		}

		return f
	}

	var baseline []string

	install(t, seams(&baseline))

	_, _, err := runIn(t, resizedManifest(), Options{NonInteractive: true, Detach: true})
	require.NoError(t, err)
	require.NotEmpty(t, baseline, "the fixture has to reach the resize for the comparison to mean anything")

	var accepted []string

	install(t, seams(&accepted))

	result, _, err := runIn(t, resizedManifest(), Options{
		Detach:       true,
		ConfirmApply: func(string) (bool, error) { return true, nil },
	})

	require.NoError(t, err)
	assert.Equal(t, baseline, accepted, "accepting the gate proceeds exactly as a run without it")
	assert.Equal(t, ActionUpdated, result.Action)
}

// countingFakes wires a named marker into every seam the contract counts as a
// mutation, so a test asserting len(calls) == 0 is asserting the run touched
// none of them.
func countingFakes(calls *[]string) func(f fakes) fakes {
	mark := func(name string) { *calls = append(*calls, name) }

	return func(f fakes) fakes {
		f.cred = func(string) (*workload.Credential, error) { mark("cred"); return nil, nil }
		f.findCredential = func(string, int) (*workload.Credential, error) { mark("findCredential"); return nil, nil }
		f.guard = func(string) error { mark("guard"); return nil }
		f.lock = func(id string) (*workload.Artifact, error) {
			mark("lock:" + id)

			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		}
		f.create = func(any) (*workload.Workload, error) { mark("create"); return running("wl-new"), nil }
		f.start = func(string) (*workload.WorkloadOperationResponse, error) { mark("start"); return nil, nil }
		f.writeID = func(string, string) error { mark("writeID"); return nil }
		f.build = func(string) (*workload.BuildTriggerResponse, error) { mark("build"); return nil, nil }
		f.codeRef = func(string, string, string) error { mark("codeRef"); return nil }
		f.sync = func(string) (*sync.Result, error) { mark("sync"); return nil, nil }
		f.settings = func(string, json.RawMessage) (*workload.Replacement, error) { mark("resize"); return nil, nil }
		f.replace = func(string, string, json.RawMessage) (*workload.Replacement, error) { mark("replace"); return nil, nil }

		return f
	}
}

// TestRun_ConfirmDecline_MutatesNothing is the promise the gate exists to
// keep: every decline answer stops the run with ErrDeclined and not one
// mutating seam has fired. The plan is printed first -- the user is declining
// something they read -- and the action stays "nothing". An answer that
// cannot be read at all is a different failure: the run stops too, but it is
// not a decline, so the sentinel stays out of it.
func TestRun_ConfirmDecline_MutatesNothing(t *testing.T) {
	cases := []struct {
		name     string
		answer   bool
		readErr  error
		declined bool
	}{
		{name: "no", answer: false, declined: true},
		{name: "unreadable", readErr: errors.New("stdin closed")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []string

			f := countingFakes(&calls)(liveDocs(t))

			install(t, f)

			result, stderr, err := runIn(t, resizedManifest(), Options{
				ConfirmApply: func(string) (bool, error) { return c.answer, c.readErr },
			})

			require.Error(t, err)

			if c.declined {
				require.ErrorIs(t, err, ErrDeclined)
			} else {
				require.NotErrorIs(t, err, ErrDeclined,
					"a failed read is not a decline, and must not read as one")
				assert.Contains(t, err.Error(), "stdin closed")
			}

			assert.Empty(t, calls, "neither answer reaches any mutating seam")
			assert.Contains(t, stderr, "-> ", "the plan precedes the question")
			assert.Equal(t, ActionUnchanged, result.Action)
		})
	}
}

// TestRun_ConfirmDecline_NoApplyMutation names the apply half of the
// non-mutation promise explicitly: on a non-empty plan, declining stops
// before the credential checks, the rollout guard and the apply itself.
func TestRun_ConfirmDecline_NoApplyMutation(t *testing.T) {
	var calls []string

	f := countingFakes(&calls)(liveDocs(t))
	f.replace = func(string, string, json.RawMessage) (*workload.Replacement, error) {
		calls = append(calls, "replace")

		return nil, nil
	}

	install(t, f)

	_, _, err := runIn(t, resizedManifest(), Options{
		ConfirmApply: func(string) (bool, error) { return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)

	for _, seam := range []string{"cred", "findCredential", "guard", "resize", "replace", "create", "build"} {
		assert.NotContains(t, calls, seam, "%s must not fire past a declined gate", seam)
	}
}

// TestRun_ConfirmDecline_NoBuild: a genuine build is a mutation like any
// other, and a decline leaves the image untriggered.
func TestRun_ConfirmDecline_NoBuild(t *testing.T) {
	var calls []string

	install(t, countingFakes(&calls)(fakes{}))

	_, _, err := runIn(t, unboundDockerfileManifest, Options{
		ConfirmApply: func(string) (bool, error) { return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.NotContains(t, calls, "build")
	assert.NotContains(t, calls, "create")
}

// TestRun_ConfirmDecline_NoLockOnlyMutation: an empty plan with --lock would
// still make the serving artifact permanent, and a decline has to stop before
// that one-way door too.
func TestRun_ConfirmDecline_NoLockOnlyMutation(t *testing.T) {
	var calls []string

	f := countingFakes(&calls)(fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return draftArtifact(t), nil },
	})

	install(t, f)

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, _, err := runIn(t, bound, Options{
		Lock:         true,
		ConfirmApply: func(string) (bool, error) { return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.NotContains(t, calls, "lock", "locking cannot be undone, so it waits for a yes like everything else")
	assert.NotContains(t, calls, "guard")
}

// TestRun_Confirm_LockOnlyPrompts: --lock is about the end state, so an empty
// plan with the flag pending still locks. Locking is a mutation, so the gate
// fires on it exactly as on a deploy: asked once, respected on both sides.
func TestRun_Confirm_LockOnlyPrompts(t *testing.T) {
	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	lockSeams := func(locked *[]string) fakes {
		return fakes{
			workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
			artifactD: func(string) (workload.Document, error) { return draftArtifact(t), nil },
			lock: func(id string) (*workload.Artifact, error) {
				*locked = append(*locked, id)

				return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
			},
		}
	}

	var declined []string

	asked := 0

	install(t, lockSeams(&declined))

	_, _, err := runIn(t, bound, Options{
		Lock: true,
		ConfirmApply: func(string) (bool, error) {
			asked++

			return false, nil
		},
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.Equal(t, 1, asked, "an empty plan with a pending lock still prompts")
	assert.Empty(t, declined, "the decline leaves the artifact as it is")

	var accepted []string

	asked = 0

	install(t, lockSeams(&accepted))

	result, _, err := runIn(t, bound, Options{
		Lock: true,
		ConfirmApply: func(string) (bool, error) {
			asked++

			return true, nil
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, asked)
	assert.Len(t, accepted, 1, "accepting proceeds into the lock")
	assert.True(t, result.Locked)
}

// TestRun_ConfirmDetach: --detach changes when the run returns, not whether
// it deploys, so the gate fires exactly as without it -- after the plan,
// before the apply. A decline still mutates nothing, and an accept goes out
// detached: the resize is requested and the run returns without waiting for
// it to serve.
func TestRun_ConfirmDetach(t *testing.T) {
	var (
		calls   []string
		resized bool
	)

	counting := func(f fakes) fakes {
		f.settings = func(string, json.RawMessage) (*workload.Replacement, error) {
			calls = append(calls, "resize")
			resized = true

			return nil, nil
		}
		f.waitSteady = func(string, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			t.Fatal("--detach must not wait for the workload to settle")

			return nil, nil
		}

		return f
	}

	install(t, counting(liveDocs(t)))

	asked := 0

	_, _, err := runIn(t, resizedManifest(), Options{
		Detach:       true,
		ConfirmApply: func(string) (bool, error) { asked++; return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.Equal(t, 1, asked, "the gate is asked on the detached path too")
	assert.Empty(t, calls, "a decline on a detached run mutates nothing")

	install(t, counting(liveDocs(t)))

	result, _, err := runIn(t, resizedManifest(), Options{
		Detach:       true,
		ConfirmApply: func(string) (bool, error) { return true, nil },
	})

	require.NoError(t, err)
	assert.True(t, resized, "accepting proceeds into the detached apply")
	assert.Equal(t, ActionUpdated, result.Action)
}

// TestRun_Confirm_NoOpDoesNotPrompt: the gate fires whenever the run would
// otherwise mutate, and a wholly empty plan would not. Its output must not
// drift either way: --confirm on a no-op run is indistinguishable from a run
// without the flag.
func TestRun_Confirm_NoOpDoesNotPrompt(t *testing.T) {
	f := liveDocs(t)
	f.create = func(any) (*workload.Workload, error) {
		t.Fatal("nothing changed, so nothing should be created")

		return nil, nil
	}

	install(t, f)

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	_, baseline, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)

	install(t, f)

	var asked bool

	_, stderr, err := runIn(t, bound, Options{
		NonInteractive: true,
		ConfirmApply:   func(string) (bool, error) { asked = true; return false, nil },
	})

	require.NoError(t, err)
	assert.False(t, asked, "nothing pending means nothing to consent to")
	assert.Equal(t, baseline, stderr, "the empty-plan output does not drift when --confirm is given")
}

// TestRun_Confirm_UnmanagedOnlyDoesNotPrompt: unmanaged fields are never
// mutated -- they survive every deploy untouched -- so a plan whose only
// content is unmanaged fields mutates nothing and must not arm the gate. The
// fixture is the empty-plan one, whose live object carries a sidecar, a
// resource bundle and an autoscaling policy the file never names.
func TestRun_Confirm_UnmanagedOnlyDoesNotPrompt(t *testing.T) {
	install(t, liveDocs(t))

	bound := "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest

	result, baseline, err := runIn(t, bound, Options{NonInteractive: true})
	require.NoError(t, err)
	require.NotEmpty(t, result.Plan.Unmanaged,
		"the fixture has to carry unmanaged fields for this to test the invariant")

	install(t, liveDocs(t))

	var asked bool

	gated, gatedStderr, err := runIn(t, bound, Options{
		NonInteractive: true,
		ConfirmApply:   func(string) (bool, error) { asked = true; return false, nil },
	})

	require.NoError(t, err)
	assert.Empty(t, gated.Plan.Artifact, "the plan under test stays empty of managed changes")
	assert.False(t, asked, "unmanaged fields do not arm the gate: nothing would change")
	assert.Equal(t, baseline, gatedStderr, "the unmanaged-only output does not drift")
}

// TestRun_ConfirmFirstDeploy: a first deploy is the loudest thing the gate can
// be asked about, so the create path prompts too, and a decline leaves the
// platform without a workload.
func TestRun_ConfirmFirstDeploy(t *testing.T) {
	var created []string

	createSeams := func() fakes {
		return fakes{
			create: func(any) (*workload.Workload, error) {
				created = append(created, "create")

				return running("wl-new"), nil
			},
			wait: func(string, workload.Serving, time.Duration, time.Duration,
				func(*workload.Workload),
			) (*workload.Workload, error) {
				return running("wl-new"), nil
			},
		}
	}

	install(t, createSeams())

	asked := 0

	_, _, err := runIn(t, unboundImageManifest, Options{
		ConfirmApply: func(string) (bool, error) { asked++; return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.Equal(t, 1, asked)
	assert.Empty(t, created, "a declined first deploy creates nothing")

	install(t, createSeams())

	result, _, err := runIn(t, unboundImageManifest, Options{
		ConfirmApply: func(string) (bool, error) { asked++; return true, nil },
	})

	require.NoError(t, err)
	assert.Equal(t, 2, asked)
	assert.Equal(t, []string{"create"}, created, "accepting proceeds into the create")
	assert.Equal(t, "wl-new", result.WorkloadID)
}

// TestRun_ConfirmWithoutDiff_UsesDefaultRender: --confirm does not require
// --diff. The default plan renders as it always does, then the gate asks; the
// diff renderer stays out of a run that never asked for it.
func TestRun_ConfirmWithoutDiff_UsesDefaultRender(t *testing.T) {
	install(t, liveDocs(t))

	var asked bool

	_, stderr, err := runIn(t, resizedManifest(), Options{
		ConfirmApply: func(string) (bool, error) { asked = true; return false, nil },
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.True(t, asked)
	assert.Contains(t, stderr, "resourceAllocation.memory: 22GB -> 24GB",
		"the default plan states a change as have -> want")
	assert.NotContains(t, stderr, "\n+ ")
	assert.NotContains(t, stderr, "identical lines", "the diff rendering is opt-in and stays opt-in")
}

// TestRun_ConfirmWithDiff_PromptsAfterTheDiff: the composition renders the
// unified diff, then asks. The diff body is what the user is consenting to,
// so it precedes the question by construction.
func TestRun_ConfirmWithDiff_PromptsAfterTheDiff(t *testing.T) {
	install(t, liveDocs(t))

	var printed int

	var stderr bytes.Buffer

	_, err := runConfirmIn(t, resizedManifest(), Options{
		Diff:         true,
		ConfirmApply: func(string) (bool, error) { printed = stderr.Len(); return false, nil },
	}, &stderr)

	require.ErrorIs(t, err, ErrDeclined)

	body := stderr.String()[:printed]

	assert.Contains(t, body, "+ containerGroups[default].containers[vllm-server].resourceAllocation.memory: 24GB",
		"the diff precedes the question")
	assert.NotContains(t, stderr.String()[printed:], "+ ",
		"nothing renders past the gate on a decline")
}

// TestRun_ConfirmAndTypedConfirm_Coexist: the two questions are different
// questions about different things, and they keep their order. The y/N asks
// whether to apply at all; the typed one asks who is rolling production. A
// decline at the y/N never reaches the second question, because there is
// nothing left to roll.
func TestRun_ConfirmAndTypedConfirm_Coexist(t *testing.T) {
	var (
		tr        track
		gateAsk   int
		typedAsk  int
		gateFirst bool
	)

	f := lockedLive(wiredRoll(&tr))
	f.guard = func(string) error {
		tr.steps = append(tr.steps, "guard")

		return nil
	}

	install(t, f)

	// Declining the y/N: the typed question is never asked and nothing runs.
	_, _, err := runIn(t, newImage(), Options{
		ConfirmApply: func(string) (bool, error) { gateAsk++; return false, nil },
		Confirm: func(string, string) (bool, error) {
			typedAsk++

			return false, nil
		},
	})

	require.ErrorIs(t, err, ErrDeclined)
	assert.Equal(t, 1, gateAsk)
	assert.Equal(t, 0, typedAsk, "a declined y/N skips the typed confirm entirely")
	assert.NotContains(t, tr.steps, "lock:art-2")
	assert.NotContains(t, tr.steps, "replace:art-2")

	// Accepting the y/N hands the run to the typed confirm, which refuses:
	// the roll never starts and nothing is locked.
	gateAsk, typedAsk = 0, 0

	_, _, err = runIn(t, newImage(), Options{
		ConfirmApply: func(string) (bool, error) {
			gateFirst = typedAsk == 0

			gateAsk++

			return true, nil
		},
		Confirm: func(string, string) (bool, error) {
			typedAsk++

			return false, nil
		},
	})

	require.Error(t, err)
	assert.True(t, gateFirst, "the y/N is asked before the typed confirm")
	assert.NotContains(t, tr.steps, "lock:art-2", "a failed typed confirm mutates nothing")
	assert.NotContains(t, tr.steps, "replace:art-2")
	assert.NotContains(t, tr.steps, "create-artifact")
}
