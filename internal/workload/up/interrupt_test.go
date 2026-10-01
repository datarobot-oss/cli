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
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A polling phase hands its function a context that dies with the phase. The
// phase is what Ctrl-C ends, and ending it is the only handle anybody has on
// the wait it left running: Bubble Tea keeps the goroutine alive, so without
// this the abandoned poll went on calling the API until the process exited.
func TestReporterWait_EndsThePhaseContextOnTheWayOut(t *testing.T) {
	fixedClock(t, time.Second)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "phase succeeds"},
		{name: "phase fails", err: errors.New("the rollout failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				out    bytes.Buffer
				handed context.Context
			)

			err := newReporter(&out, false).wait(t.Context(), "Waiting for the rollout",
				func(ctx context.Context, _ tui.Noter) error {
					handed = ctx

					require.NoError(t, ctx.Err(), "the phase starts with a live context")

					return tc.err
				})

			assert.Equal(t, tc.err, err)

			require.NotNil(t, handed)
			assert.ErrorIs(t, handed.Err(), context.Canceled,
				"the phase's context outlived the phase, so an abandoned wait keeps polling")
		})
	}
}

// The checkmark is the claim that the phase finished. A phase that returned an
// error must not print one, whatever the error was: a tick above an error
// message reads as though both had happened.
func TestReporterWait_NoCheckmarkForAnInterruptedPhase(t *testing.T) {
	fixedClock(t, time.Second)

	var out bytes.Buffer

	err := newReporter(&out, false).wait(t.Context(), "Waiting for the new version to serve",
		func(context.Context, tui.Noter) error { return tui.ErrInterrupted })

	require.ErrorIs(t, err, tui.ErrInterrupted)
	assert.NotContains(t, out.String(), "✓")
}

// progress used to return nil whenever a spinner was drawing, which left the
// one reader who is definitely watching with nothing but a moving glyph for
// the longest phase of the deploy. On that path it now writes the spinner's
// own label suffix.
func TestProgress_SpeaksOnBothPaths(t *testing.T) {
	fixedClock(t, 30*time.Second)

	t.Run("with a spinner it notes instead of printing", func(t *testing.T) {
		var out bytes.Buffer

		report := newReporter(&out, true)
		opts := Options{Spinner: true, PollInterval: time.Second}

		var noted string

		tick := progress("waiting for the rollout", opts, report, func(s string) { noted = s })
		require.NotNil(t, tick, "the spinner path is exactly where somebody is watching")

		tick(&workload.Workload{Status: workload.WorkloadStatusRunning})

		assert.Contains(t, noted, "so far")
		assert.Contains(t, noted, workload.WorkloadStatusRunning)
		assert.Empty(t, out.String(), "the note belongs in the label, not on its own line")
	})

	t.Run("without one it prints a line", func(t *testing.T) {
		var out bytes.Buffer

		report := newReporter(&out, false)
		opts := Options{PollInterval: heartbeatEvery}

		tick := progress("waiting for the rollout", opts, report, func(string) {
			t.Fatal("there is no spinner to write a note on")
		})
		require.NotNil(t, tick)

		tick(&workload.Workload{Status: workload.WorkloadStatusRunning})

		assert.Contains(t, out.String(), "waiting for the rollout")
		assert.Contains(t, out.String(), "so far")
	})
}

// held replaces the captured variable each polling phase used to publish what
// it saw. The point is that the write and the read no longer race, which is
// what -race is here to say.
func TestHeld_SurvivesAWriterThatOutlivesTheReader(t *testing.T) {
	var (
		h    held[workload.Workload]
		done sync.WaitGroup
	)

	assert.Nil(t, h.get(), "a phase that never got an answer holds nothing")

	done.Add(1)

	go func() {
		defer done.Done()

		h.set(&workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusRunning})
	}()

	// Reading while the abandoned goroutine writes is the shape Ctrl-C leaves
	// behind.
	_ = h.get()

	done.Wait()

	require.NotNil(t, h.get())
	assert.Equal(t, "wl-1", h.get().ID)
}

// The whole point of the change, at the level a user meets it: a wait that was
// interrupted reports a failure, and claims nothing about where the rollout
// got to.
//
// The stub returns a workload beside the error, which is what the real poll
// loop does — pollWorkload hands back its last successful read alongside the
// cancellation. A stub returning nil there would make this pass for a reason
// the production path does not have.
//
// Both ways of stopping are held: the signal a piped run gets, which arrives
// as the context's cancellation, and the keystroke the terminal UI catches,
// which arrives as tui.ErrInterrupted with the context still live. Checking
// only the first would let `!Interrupted(err)` become a bare context check
// and quietly report the outgoing version for the second.
func TestAwaitRunning_AnInterruptedWaitIsNotASuccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancelled bool
		fail      func(ctx context.Context) error
		want      error
	}{
		{
			name:      "signal: the context is cancelled",
			cancelled: true,
			fail:      abandonedLikeThePollLoop,
			want:      context.Canceled,
		},
		{
			name: "keystroke: the spinner was quit, the context is still live",
			fail: func(context.Context) error { return tui.ErrInterrupted },
			want: tui.ErrInterrupted,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixedClock(t, time.Second)

			// The rollout mid-flight: the workload still reports running, on
			// the artifact being replaced, at the endpoint the old version
			// answers.
			midRoll := &workload.Workload{
				ID:         "wl-1",
				Status:     workload.WorkloadStatusRunning,
				ArtifactID: "art-outgoing",
				Endpoint:   "https://example.invalid/old",
			}

			var out bytes.Buffer

			swap(t, &waitWorkloadFn, func(ctx context.Context, _ string, _ workload.Serving,
				_, _ time.Duration, _ func(*workload.Workload),
			) (*workload.Workload, error) {
				return midRoll, tc.fail(ctx)
			})

			ctx, cancel := context.WithCancel(t.Context())
			if tc.cancelled {
				cancel()
			} else {
				defer cancel()
			}

			report := newReporter(&out, false)
			opts := Options{Stderr: &out, PollInterval: time.Millisecond, PollTimeout: time.Minute}

			result, err := awaitRunning(ctx, "wl-1", workload.Serving{ArtifactID: "art-new"},
				Result{WorkloadID: "wl-1"}, opts, report)

			require.ErrorIs(t, err, tc.want, "an abandoned wait must never come back nil")

			// None of the outgoing version's details are recorded. Reporting
			// them would put the artifact being rolled off, under a status
			// that reads as arrived, into the summary and the JSON envelope
			// of a deploy nobody waited for.
			assert.Empty(t, result.Status)
			assert.Empty(t, result.Endpoint)
			assert.NotEqual(t, "art-outgoing", result.ArtifactID)
			assert.NotContains(t, out.String(), "✓")
		})
	}
}

// A timeout is the other half of the same decision: that wait ran the whole
// way, so where it got to is the finding and still gets reported.
func TestAwaitRunning_ATimeoutStillReportsWhereItGotTo(t *testing.T) {
	fixedClock(t, time.Second)

	stalled := &workload.Workload{
		ID:         "wl-1",
		Status:     workload.WorkloadStatusRunning,
		ArtifactID: "art-outgoing",
		Endpoint:   "https://example.invalid/old",
	}

	var out bytes.Buffer

	swap(t, &waitWorkloadFn, func(context.Context, string, workload.Serving,
		time.Duration, time.Duration, func(*workload.Workload),
	) (*workload.Workload, error) {
		return stalled, errors.New("timeout waiting for workload wl-1 after 30m0s")
	})

	report := newReporter(&out, false)
	opts := Options{Stderr: &out, PollInterval: time.Millisecond, PollTimeout: time.Minute}

	result, err := awaitRunning(t.Context(), "wl-1", workload.Serving{ArtifactID: "art-new"},
		Result{WorkloadID: "wl-1"}, opts, report)

	require.Error(t, err)
	assert.Equal(t, workload.WorkloadStatusRunning, result.Status)
	assert.Equal(t, "art-outgoing", result.ArtifactID)
}

// abandonedLikeThePollLoop wraps the context's error the way the package's
// poll loops do, so the shape the caller inspects is the real one.
func abandonedLikeThePollLoop(ctx context.Context) error {
	return fmt.Errorf("stopped waiting for workload wl-1: %w", ctx.Err())
}

// Ctrl-C during the endpoint check lands after the rollout finished, so the
// deploy is not failed over it. But --lock is the one irreversible step left,
// and taking it after the user asked the run to stop is the opposite of what
// the keystroke meant: the lock is withheld, said out loud, and the summary
// carries locked=false.
func TestFinishSettle_AnInterruptedEndpointCheckWithholdsTheLock(t *testing.T) {
	var locked int

	swap(t, &lockArtifactFn, func(string) (*workload.Artifact, error) {
		locked++

		return &workload.Artifact{ID: "art-1"}, nil
	})

	var out bytes.Buffer

	report := newReporter(&out, false)
	running := Result{WorkloadID: "wl-1", ArtifactID: "art-1", Status: workload.WorkloadStatusRunning}

	result, err := finishSettle(running, true, Options{Lock: true, Stderr: &out}, report)
	require.NoError(t, err, "the rollout finished; the deploy is a success")
	assert.False(t, result.Locked)
	assert.Zero(t, locked, "the irreversible step was taken after the user asked to stop")
	assert.Contains(t, out.String(), "--lock skipped: interrupted")
	assert.Contains(t, out.String(), "dr workload up --lock")

	// The other half: an uninterrupted check locks as asked.
	result, err = finishSettle(running, false, Options{Lock: true, Stderr: &out}, report)
	require.NoError(t, err)
	assert.True(t, result.Locked)
	assert.Equal(t, 1, locked)
}

// The three waits besides awaitRunning that a keystroke or a signal can end.
// Each hands its poll the phase's context and returns the interrupt rather
// than swallowing it; awaitSteady matters most, because a nil there lets the
// deploy carry on into planning after Ctrl-C. A stub that ignored the context
// it was given would pass with context.Background() wired in, so each one
// derives its error from the context it actually received.
func TestOtherWaits_AnInterruptedWaitIsNotASuccess(t *testing.T) {
	fixedClock(t, time.Second)

	promoting := &workload.Replacement{ID: "rep-1", Status: "promoting"}

	stubReplacement := func(t *testing.T) {
		t.Helper()

		swap(t, &waitReplacementFn, func(ctx context.Context, _ string, _ *workload.Replacement,
			_, _ time.Duration, _ func(*workload.Replacement),
		) (*workload.Replacement, error) {
			return promoting, fmt.Errorf("stopped waiting for replacement rep-1: %w", ctx.Err())
		})
	}

	for _, tc := range []struct {
		name string
		stub func(t *testing.T)
		wait func(ctx context.Context, opts Options, report *reporter) error
	}{
		{
			name: "awaitSteady",
			stub: func(t *testing.T) {
				t.Helper()

				swap(t, &waitSteadyFn, func(ctx context.Context, _ string,
					_, _ time.Duration, _ func(*workload.Workload),
				) (*workload.Workload, error) {
					return &workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusStopping},
						abandonedLikeThePollLoop(ctx)
				})
			},
			wait: func(ctx context.Context, opts Options, _ *reporter) error {
				live := Live{State: StateSettling, Status: workload.WorkloadStatusStopping}
				live.WorkloadID = "wl-1"

				_, err := awaitSteady(ctx, live, opts)

				return err
			},
		},
		{
			name: "awaitRollout",
			stub: stubReplacement,
			wait: func(ctx context.Context, opts Options, report *reporter) error {
				return awaitRollout(ctx, "wl-1", promoting, opts, report)
			},
		},
		{
			name: "awaitResize",
			stub: stubReplacement,
			wait: func(ctx context.Context, opts Options, report *reporter) error {
				return awaitResize(ctx, "wl-1", promoting, opts, report)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.stub(t)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			var out bytes.Buffer

			opts := Options{Stderr: &out, PollInterval: time.Millisecond, PollTimeout: time.Minute}

			err := tc.wait(ctx, opts, newReporter(&out, false))

			require.ErrorIs(t, err, context.Canceled, "an abandoned wait must never come back nil")
			assert.True(t, Interrupted(err))
			assert.NotContains(t, out.String(), "✓", "an interrupted phase must not be check-marked")
		})
	}
}
