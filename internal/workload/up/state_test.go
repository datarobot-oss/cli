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
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateFor(t *testing.T) {
	cases := []struct {
		status string
		want   State
	}{
		{workload.WorkloadStatusRunning, StateRunning},
		{workload.WorkloadStatusStopped, StateStopped},
		{workload.WorkloadStatusSuspended, StateStopped},
		{workload.WorkloadStatusInterrupted, StateStopped},
		{workload.WorkloadStatusTerminated, StateTerminated},
		{workload.WorkloadStatusErrored, StateErrored},
		{workload.WorkloadStatusSubmitted, StateSettling},
		{workload.WorkloadStatusProvisioning, StateSettling},
		{workload.WorkloadStatusLaunching, StateSettling},
		{workload.WorkloadStatusStopping, StateSettling},
		{workload.WorkloadStatusUnknown, StateSettling},
		{"a-status-the-platform-added-later", StateSettling},
		{"", StateSettling},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, stateFor(c.status), "status %q", c.status)
	}
}

// TestStateFor_EveryPlatformStatusIsMapped fails if the platform's status
// enum grows and nobody revisits this file. The default arm means a new
// status silently becomes StateSettling, which is the safe answer but not
// necessarily the right one, so the reminder is worth having.
func TestStateFor_EveryPlatformStatusIsMapped(t *testing.T) {
	known := []string{
		workload.WorkloadStatusUnknown,
		workload.WorkloadStatusSubmitted,
		workload.WorkloadStatusProvisioning,
		workload.WorkloadStatusLaunching,
		workload.WorkloadStatusRunning,
		workload.WorkloadStatusSuspended,
		workload.WorkloadStatusInterrupted,
		workload.WorkloadStatusStopping,
		workload.WorkloadStatusStopped,
		workload.WorkloadStatusErrored,
		workload.WorkloadStatusTerminated,
	}

	assert.Len(t, known, 11, "the platform's status enum changed; revisit stateFor")

	for _, status := range known {
		assert.NotEmpty(t, stateFor(status).String(), "status %q maps to a nameless state", status)
	}
}

// The statuses that mean "off" reduce to one state, because the plan is the
// same for all three: the workload is not serving and `up` has to say so.
// Which of them can actually be started is a question for the apply, and is
// answered there rather than by the reduction.
func TestState_TheOffStatusesReduceToStopped(t *testing.T) {
	for _, status := range []string{
		workload.WorkloadStatusStopped,
		workload.WorkloadStatusSuspended,
		workload.WorkloadStatusInterrupted,
	} {
		assert.Equal(t, StateStopped, stateFor(status), "status %q", status)
	}
}

func TestState_String(t *testing.T) {
	assert.Equal(t, "not created yet", StateUnbound.String(),
		"the envelope reports this; scripts match on it, so the header wording lives in render.go")
	assert.Equal(t, "missing", StateMissing.String())
	assert.Equal(t, "terminated", StateTerminated.String())
	assert.Equal(t, "stopped", StateStopped.String())
	assert.Equal(t, "settling", StateSettling.String())
	assert.Equal(t, "running", StateRunning.String())
	assert.Equal(t, "errored", StateErrored.String())
	assert.Equal(t, "unknown", State(99).String())
}

// The platform does not agree with itself about the casing of its status
// enums. Read exactly, a workload answering "RUNNING" falls to the default and
// is reported as still settling, so every deploy onto a healthy workload is
// refused as unsettled.
func TestStateFor_FoldsCase(t *testing.T) {
	for _, c := range []struct {
		status string
		want   State
	}{
		{"RUNNING", StateRunning},
		{"Running", StateRunning},
		{"STOPPED", StateStopped},
		{"Suspended", StateStopped},
		{"INTERRUPTED", StateStopped},
		{"TERMINATED", StateTerminated},
		{"Errored", StateErrored},
		{"LAUNCHING", StateSettling},
	} {
		assert.Equal(t, c.want, stateFor(c.status), "status %q", c.status)
	}
}

// startable reads the same status stateFor just folded, so an upper-case
// suspended must still be refused. Left exact it would be treated as startable
// and polled until the timeout for a transition the platform never makes.
func TestStartable_FoldsSuspended(t *testing.T) {
	err := startable(Live{Status: "SUSPENDED"}, "my-app")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "suspended")

	require.NoError(t, startable(Live{Status: "STOPPED"}, "my-app"),
		"stopped and interrupted can still be started")
}

// The status a detached start reports is mapped the same way.
func TestStartingStatus_FoldsTheStatesItMaps(t *testing.T) {
	assert.Equal(t, workload.WorkloadStatusSubmitted, startingStatus("STOPPED"))
	assert.Equal(t, workload.WorkloadStatusSubmitted, startingStatus("Interrupted"))
	assert.Equal(t, "something-new", startingStatus("something-new"),
		"an unrecognised status is still passed through rather than overwritten")
}
