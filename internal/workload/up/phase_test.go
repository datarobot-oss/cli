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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedClock makes elapsed times deterministic by advancing the clock by step
// on every read, so a phase always appears to take exactly step.
func fixedClock(t *testing.T, step time.Duration) {
	t.Helper()

	prev := phaseClock
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)

	phaseClock = func() time.Time {
		current := now
		now = now.Add(step)

		return current
	}

	t.Cleanup(func() { phaseClock = prev })
}

func TestReporter_CheckMarksASuccessfulPhase(t *testing.T) {
	fixedClock(t, 2*time.Second)

	var out bytes.Buffer

	require.NoError(t, newReporter(&out, false).run("Creating workload", func() error { return nil }))
	assert.Equal(t, "  ✓ Creating workload (2s)\n", out.String())
}

// TestReporter_FailureIsSilent: a checkmark followed by an error message
// reads as though both happened. The error carries the story on its own.
func TestReporter_FailureIsSilent(t *testing.T) {
	fixedClock(t, time.Second)

	var out bytes.Buffer

	err := newReporter(&out, false).run("Creating workload", func() error { return errors.New("refused") })
	require.Error(t, err)
	assert.Empty(t, out.String())
}

// TestReporter_DurationsAreRounded keeps a deploy's timings readable. It is
// measured in minutes, so microseconds are noise.
func TestReporter_DurationsAreRounded(t *testing.T) {
	fixedClock(t, 1234*time.Millisecond)

	var out bytes.Buffer

	require.NoError(t, newReporter(&out, false).run("Waiting", func() error { return nil }))
	assert.Contains(t, out.String(), "(1.2s)")
}

// TestReporter_WritesToTheInjectedWriter is the whole reason this exists
// rather than calling tui.RunWithSpinner directly: that writes to os.Stderr,
// which escapes both test capture and any redirection the command set up.
func TestReporter_WritesToTheInjectedWriter(t *testing.T) {
	fixedClock(t, time.Second)

	var out bytes.Buffer

	report := newReporter(&out, false)
	report.say("note: %s\n", "something happened")

	require.NoError(t, report.run("Phase", func() error { return nil }))

	assert.Contains(t, out.String(), "note: something happened")
	assert.Contains(t, out.String(), "✓ Phase")
}

func TestReporter_RunsTheWorkExactlyOnce(t *testing.T) {
	fixedClock(t, time.Second)

	var (
		out   bytes.Buffer
		calls int
	)

	require.NoError(t, newReporter(&out, false).run("Phase", func() error {
		calls++

		return nil
	}))
	assert.Equal(t, 1, calls)
}
