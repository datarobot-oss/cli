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
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporter_StreamPrintsHeaderLinesAndCheckmark(t *testing.T) {
	fixedClock(t, 2*time.Second)

	var out bytes.Buffer

	err := newReporter(&out, false).stream("Building the image", func(say func(string, lipgloss.Style)) error {
		say("step 1/4: FROM base", tui.HintStyle)
		say("step 2/4: COPY . .", tui.HintStyle)

		return nil
	})
	require.NoError(t, err)

	text := out.String()
	assert.Contains(t, text, "Building the image\n")
	assert.Contains(t, text, "step 1/4: FROM base")
	assert.Contains(t, text, "step 2/4: COPY . .")
	assert.Contains(t, text, "✓ Building the image (2s)")

	// Header before lines, lines before checkmark.
	header := bytes.Index(out.Bytes(), []byte("Building the image"))
	line := bytes.Index(out.Bytes(), []byte("step 1/4"))
	check := bytes.Index(out.Bytes(), []byte("✓"))

	assert.Less(t, header, line)
	assert.Less(t, line, check)
}

func TestReporter_StreamFailurePrintsNoCheckmark(t *testing.T) {
	fixedClock(t, time.Second)

	var out bytes.Buffer

	err := newReporter(&out, false).stream("Building the image", func(say func(string, lipgloss.Style)) error {
		say("step 1/4: FROM base", tui.HintStyle)

		return errors.New("boom")
	})
	require.Error(t, err)
	assert.NotContains(t, out.String(), "✓")
	// The streamed lines stay: they are the context for the error.
	assert.Contains(t, out.String(), "step 1/4: FROM base")
}

func TestBuildLogLine_MarksAndColorsNonInfoLevels(t *testing.T) {
	line, style := buildLogLine(workload.WorkloadLogEntry{Level: "INFO", Message: "pulling layer"})
	assert.Equal(t, "pulling layer", line)
	assert.Equal(t, tui.HintStyle, style)

	line, style = buildLogLine(workload.WorkloadLogEntry{Level: "error", Message: "no space left on device"})
	assert.Equal(t, "[ERROR] no space left on device", line)
	assert.Equal(t, tui.ErrorStyle, style)

	line, style = buildLogLine(workload.WorkloadLogEntry{Level: "warning", Message: "deprecated base image"})
	assert.Equal(t, "[WARNING] deprecated base image", line)
	assert.Equal(t, tui.WarnStyle, style)
}

func TestBuildStatusLine_NarratesTheQuietStages(t *testing.T) {
	assert.Equal(t, "build accepted; waiting for a builder to pick it up", buildStatusLine("PENDING"))
	assert.Equal(t, "builder running", buildStatusLine("IN_PROGRESS"))
	assert.Equal(t, "image built; pushing it to the registry", buildStatusLine("BUILT"))
	assert.Equal(t, "build is cancelling", buildStatusLine("CANCELLING"))
}

// terminalOfSize fakes the stream's terminal for the test's lifetime.
func terminalOfSize(t *testing.T, width, height int) {
	t.Helper()

	prev := streamSize
	streamSize = func() (int, int) { return width, height }

	t.Cleanup(func() { streamSize = prev })
}

func TestReporter_StreamCollapsesOnSuccessOnTerminal(t *testing.T) {
	fixedClock(t, 2*time.Second)
	terminalOfSize(t, 30, 24)

	var out bytes.Buffer

	// spinner=true marks out as the user's terminal, which is what arms
	// truncation, the sliding window, and the erase.
	err := newReporter(&out, true).stream("Building the image", func(say func(string, lipgloss.Style)) error {
		say("short line", tui.HintStyle)
		say("a very long line that cannot possibly fit in thirty columns", tui.HintStyle)

		return nil
	})
	require.NoError(t, err)

	text := out.String()
	// The long line was truncated to one row.
	assert.Contains(t, text, "…")
	assert.NotContains(t, text, "thirty columns")
	// Two window rows plus the header are erased, then the checkmark prints.
	assert.Contains(t, text, "\x1b[3A\x1b[0J")
	assert.Contains(t, text, "✓ Building the image (2s)")
}

// A panic inside fn unwinds through stream. The deferred halt has to stop the
// animator on the way out, or its ticker keeps writing escape codes to the
// terminal until the process dies.
func TestReporter_StreamStopsTheAnimatorWhenFnPanics(t *testing.T) {
	terminalOfSize(t, 30, 24)

	var out bytes.Buffer

	panicked := func() (p any) {
		defer func() { p = recover() }()

		_ = newReporter(&out, true).stream("Building the image", func(func(string, lipgloss.Style)) error {
			panic("boom")
		})

		return nil
	}()
	require.Equal(t, "boom", panicked, "the panic must still propagate; stream only cleans up")

	// The deferred halt ran before the panic reached us, so the animator is
	// stopped and the buffer may no longer move. A tick is 100ms; if one were
	// still alive, this window would catch it repainting the header.
	before := out.Len()

	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, before, out.Len(), "a stopped animator writes nothing more")
}

func TestReporter_StreamWindowNeverOutgrowsTheViewport(t *testing.T) {
	fixedClock(t, time.Second)
	// A viewport shorter than the default window: the region must shrink to
	// fit, or the cursor could not reach back over it (it cannot move above
	// the viewport, and scrolled rows would survive in scrollback).
	terminalOfSize(t, 40, 6)

	var out bytes.Buffer

	err := newReporter(&out, true).stream("Building the image", func(say func(string, lipgloss.Style)) error {
		for i := range 20 {
			say(fmt.Sprintf("line %d", i), tui.HintStyle)
		}

		return nil
	})
	require.NoError(t, err)

	// height-4 = 2 window rows, plus the header: the final erase is 3 rows.
	assert.Contains(t, out.String(), "\x1b[3A\x1b[0J")
	// The redraws never move the cursor further up than the window is tall.
	assert.NotContains(t, out.String(), "\x1b[4A")
}

// unchangedSync is a sync result that moved nothing, which is the only state
// the attach path is reachable from.
func unchangedSync() *sync.Result { return &sync.Result{} }

func TestMaybeBuild_AttachesToRunningBuildWhenCodeUnchanged(t *testing.T) {
	fixedClock(t, time.Second)

	// Newest build is still running and nothing has ever completed: the code
	// on the artifact is being built right now, so a second trigger would be
	// a duplicate.
	force(t, &listBuildsFn, func(string, int) ([]workload.Build, error) {
		return []workload.Build{{ID: "bld-running", Status: "IN_PROGRESS"}}, nil
	})
	force(t, &triggerBuildFn, func(string) (*workload.BuildTriggerResponse, error) {
		t.Fatal("attach must not trigger a second build")

		return nil, nil
	})

	var waited string

	force(t, &waitBuildFn, func(_, id string, _, _ time.Duration, onTick func(*workload.Build)) (*workload.Build, error) {
		waited = id

		if onTick != nil {
			onTick(&workload.Build{ID: id, Status: "IN_PROGRESS"})
		}

		return &workload.Build{ID: id, Status: "COMPLETED"}, nil
	})

	var out bytes.Buffer

	buildID, err := maybeBuild("art-1", false, CodeChange{}, unchangedSync(), Options{}, newReporter(&out, false))
	require.NoError(t, err)
	assert.Equal(t, "bld-running", buildID)
	assert.Equal(t, "bld-running", waited)
	assert.Contains(t, out.String(), "already running; attaching")
	// The heartbeat line: the phase must say something before the first log
	// line arrives, or a slow start reads as a hang.
	assert.Contains(t, out.String(), "following build bld-running")
	// The status transition the stub's onTick delivered is narrated.
	assert.Contains(t, out.String(), "builder running")
}

func TestMaybeBuild_TriggersDespiteRunningBuildWhenCodeChanged(t *testing.T) {
	fixedClock(t, time.Second)

	// A running build of the OLD tree must not be attached to: the fresh
	// trigger supersedes it server-side and builds what was just synced.
	force(t, &listBuildsFn, func(string, int) ([]workload.Build, error) {
		t.Fatal("changed code must not consult the build list")

		return nil, nil
	})

	triggered := false

	force(t, &triggerBuildFn, func(string) (*workload.BuildTriggerResponse, error) {
		triggered = true

		return &workload.BuildTriggerResponse{BuildIDs: []string{"bld-new"}}, nil
	})
	force(t, &waitBuildFn, func(_, id string, _, _ time.Duration, _ func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: id, Status: "COMPLETED"}, nil
	})

	var out bytes.Buffer

	buildID, err := maybeBuild("art-1", false, CodeChange{}, &sync.Result{UploadedCount: 1}, Options{}, newReporter(&out, false))
	require.NoError(t, err)
	assert.True(t, triggered)
	assert.Equal(t, "bld-new", buildID)
}
