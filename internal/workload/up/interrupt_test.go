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
// this the abandoned poll went on calling the API until the process exited
// (RAPTOR-19963).
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
// interrupted reports a failure, and says the rollout is still going.
func TestAwaitRunning_AnInterruptedWaitIsNotASuccess(t *testing.T) {
	fixedClock(t, time.Second)

	var out bytes.Buffer

	swap(t, &waitWorkloadFn, func(ctx context.Context, _ string, _ workload.Serving,
		_, _ time.Duration, _ func(*workload.Workload),
	) (*workload.Workload, error) {
		<-ctx.Done()

		// What the real wait does once its context ends: report where it got
		// to, and that it did not get there.
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	report := newReporter(&out, false)
	opts := Options{Context: ctx, Stderr: &out, PollInterval: time.Millisecond, PollTimeout: time.Minute}

	result, err := awaitRunning("wl-1", workload.Serving{}, Result{WorkloadID: "wl-1"}, opts, report)

	require.ErrorIs(t, err, context.Canceled, "an abandoned wait must never come back nil")

	// Nothing was learned, so nothing is claimed. Reporting the status of the
	// version being rolled off is how --lock came to lock the wrong artifact.
	assert.Empty(t, result.Status)
	assert.Empty(t, result.Endpoint)
	assert.NotContains(t, out.String(), "✓")
}

func TestOptionsCtx_DefaultsToBackground(t *testing.T) {
	assert.Equal(t, context.Background(), Options{}.ctx())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	assert.Equal(t, ctx, Options{Context: ctx}.ctx())
}
