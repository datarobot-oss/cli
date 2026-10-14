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
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/up"
	"github.com/datarobot/cli/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stopping the wait does not stop the deploy, so "interrupted" on its own
// would read as though Ctrl-C had called the rollout off. The message has to
// say the platform is still going and where to look.
func TestExplainInterrupt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		result   up.Result
		wants    []string
		unwanted string
		same     bool
	}{
		{
			name:   "keystroke, workload known",
			err:    tui.ErrInterrupted,
			result: up.Result{WorkloadID: "wl-42"},
			wants:  []string{"already submitted carries on", "dr workload status wl-42"},
		},
		{
			name:   "signal, workload known",
			err:    context.Canceled,
			result: up.Result{WorkloadID: "wl-42"},
			wants:  []string{"dr workload status wl-42"},
		},
		{
			// A first deploy stopped mid-build: no workload exists yet, and
			// the build is the thing still running. Pointing at a status
			// command for a workload that does not exist would be wrong.
			name:   "interrupted mid-build, before any workload",
			err:    tui.ErrInterrupted,
			result: up.Result{ArtifactID: "art-3", BuildID: "b-7"},
			// Both ids: the one-argument form resolves the artifact from the
			// current directory, which is not the project's under --dir.
			wants: []string{"dr artifact build logs art-3 b-7"},
			// The claim must not be specific to a rollout, since none was
			// started.
			unwanted: "rolling",
		},
		{
			name:   "interrupted mid-build with only the build id in hand",
			err:    tui.ErrInterrupted,
			result: up.Result{BuildID: "b-7"},
			wants:  []string{"dr artifact build logs b-7"},
		},
		{
			name: "interrupted before anything had an id",
			err:  tui.ErrInterrupted,
			// No id to name, so the bare command is the best that can be
			// said; it must not print a dangling "status ".
			wants:    []string{"dr workload status"},
			unwanted: "status '",
		},
		{
			name:   "an ordinary failure is left alone",
			err:    errors.New("the build failed"),
			result: up.Result{WorkloadID: "wl-42"},
			same:   true,
		},
		{
			name: "success is left alone",
			same: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := explainInterrupt(tc.err, tc.result)

			if tc.same {
				assert.Equal(t, tc.err, got)

				return
			}

			require.Error(t, got)

			for _, want := range tc.wants {
				assert.Contains(t, got.Error(), want)
			}

			if tc.unwanted != "" {
				assert.NotContains(t, got.Error(), tc.unwanted)
			}

			// The cause survives, so a caller can still tell an interrupt
			// from anything else.
			assert.ErrorIs(t, got, tc.err)
		})
	}
}

// The command has to hand the deploy the context it was given, or nothing
// below can be interrupted at all: main.go's signal.NotifyContext has already
// taken away the process's default death on SIGINT.
//
// Run through ExecuteContext with a context that is already cancelled, and
// checked by that cancellation reaching the deploy: cobra fills in a
// background context when none was given, so "not nil" would pass with
// `context.Background()` in the command and prove nothing.
func TestUpPassesTheCommandContextToTheDeploy(t *testing.T) {
	seen := stubRun(t, up.Result{WorkloadID: "wl-1", Status: "running"}, nil)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--dir", t.TempDir(), "--yes"})

	require.NoError(t, cmd.ExecuteContext(ctx))

	require.NotNil(t, seen.ctx, "a deploy with no context cannot be stopped")
	assert.ErrorIs(t, seen.ctx.Err(), context.Canceled, "the deploy was handed some other context")
}
