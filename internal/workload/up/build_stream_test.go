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

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporter_StreamPrintsHeaderLinesAndCheckmark(t *testing.T) {
	fixedClock(t, 2*time.Second)

	var out bytes.Buffer

	err := newReporter(&out, false).stream("Building the image", func(say func(string)) error {
		say("step 1/4: FROM base")
		say("step 2/4: COPY . .")

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

	err := newReporter(&out, false).stream("Building the image", func(say func(string)) error {
		say("step 1/4: FROM base")

		return errors.New("boom")
	})
	require.Error(t, err)
	assert.NotContains(t, out.String(), "✓")
	// The streamed lines stay: they are the context for the error.
	assert.Contains(t, out.String(), "step 1/4: FROM base")
}

func TestBuildLogLine_MarksNonInfoLevels(t *testing.T) {
	assert.Equal(t, "pulling layer",
		buildLogLine(workload.WorkloadLogEntry{Level: "INFO", Message: "pulling layer"}))
	assert.Equal(t, "cache key",
		buildLogLine(workload.WorkloadLogEntry{Level: "debug", Message: "cache key"}))
	assert.Equal(t, "[ERROR] no space left on device",
		buildLogLine(workload.WorkloadLogEntry{Level: "error", Message: "no space left on device"}))
	assert.Equal(t, "[WARNING] deprecated base image",
		buildLogLine(workload.WorkloadLogEntry{Level: "warning", Message: "deprecated base image"}))
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
