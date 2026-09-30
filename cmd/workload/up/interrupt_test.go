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
	"context"
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/workload/up"
	"github.com/datarobot/cli/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stopping the wait does not stop the deploy, so "interrupted" on its own
// would read as though Ctrl-C had called the rollout off. The message has to
// say the platform is still going and where to look (RAPTOR-19963).
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
			wants:  []string{"still rolling", "dr workload status wl-42"},
		},
		{
			name:   "signal, workload known",
			err:    context.Canceled,
			result: up.Result{WorkloadID: "wl-42"},
			wants:  []string{"dr workload status wl-42"},
		},
		{
			name: "interrupted before the workload had an id",
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

			// The cause survives, so a caller can still tell an interrupt
			// from anything else.
			assert.ErrorIs(t, got, tc.err)
		})
	}
}

// The command has to hand the deploy the context it was given, or nothing
// below can be interrupted at all: main.go's signal.NotifyContext has already
// taken away the process's default death on SIGINT.
func TestUpPassesTheCommandContextToTheDeploy(t *testing.T) {
	dir := t.TempDir()

	seen := stubRun(t, up.Result{WorkloadID: "wl-1", Status: "running"}, nil)

	_, _, err := runCmd(t, "--dir", dir, "--yes")
	require.NoError(t, err)

	require.NotNil(t, seen.Context, "a deploy with no context cannot be stopped")
}
