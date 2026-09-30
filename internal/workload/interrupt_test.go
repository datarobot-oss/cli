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

package workload

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The property every one of these holds: a wait whose context ended must
// report an error. Every caller reads a nil error as "it arrived", so a wait
// that gives up quietly is reported as a healthy deploy — which is the whole
// of RAPTOR-19963. The workload or build it last saw still comes back, because
// a caller that has to say where things got to needs it.

func TestWaitsRefuseToSucceedOnACancelledContext(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Whatever is asked for, answer with something a wait would happily
		// sit on, so the only thing that can end these waits is the context.
		switch {
		case wantsBuild(r):
			fmt.Fprint(w, `{"id":"b-1","artifactId":"art-1","status":"IN_PROGRESS"}`)
		case wantsReplacement(r):
			fmt.Fprint(w, `{"id":"rep-1","workloadId":"wl-1","status":"promoting"}`)
		default:
			fmt.Fprint(w, serverWorkloadDoc("wl-1", "art-1", WorkloadStatusSubmitted))
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// A generous timeout on purpose: if the context were being ignored these
	// would run for a minute rather than fail, so a hung test is itself the
	// failure report.
	const (
		interval = time.Millisecond
		timeout  = time.Minute
	)

	t.Run("WaitForWorkload", func(t *testing.T) {
		_, err := WaitForWorkload(ctx, "wl-1", Serving{}, interval, timeout, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("WaitForSteadyWorkload", func(t *testing.T) {
		_, err := WaitForSteadyWorkload(ctx, "wl-1", interval, timeout, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("WaitForBuild", func(t *testing.T) {
		_, err := WaitForBuild(ctx, "art-1", "b-1", interval, timeout, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("WaitForReplacement", func(t *testing.T) {
		_, err := WaitForReplacement(ctx, "wl-1", nil, interval, timeout, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// A wait already in flight has to notice too. Cancelling only before the first
// poll would leave the case this is actually about — Ctrl-C partway through a
// rollout — polling the API until the process exits.
func TestWaitForWorkloadStopsPollingWhenTheContextEndsMidWait(t *testing.T) {
	installSkipAuth(t)

	var polls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&polls, 1)

		// Never arrives: submitted is not terminal, so only the context or
		// the deadline can end this.
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "art-1", WorkloadStatusSubmitted))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	ctx, cancel := context.WithCancel(t.Context())

	// Cancelled from the third poll, standing in for the keystroke.
	wl, err := WaitForWorkload(ctx, "wl-1", Serving{}, time.Millisecond, time.Minute,
		func(*Workload) {
			if atomic.LoadInt32(&polls) >= 3 {
				cancel()
			}
		})

	require.ErrorIs(t, err, context.Canceled)

	// The last read comes back with the error: a caller reporting where the
	// deploy got to has nothing else to report from.
	require.NotNil(t, wl)
	assert.Equal(t, WorkloadStatusSubmitted, wl.Status)

	settled := atomic.LoadInt32(&polls)

	// Nothing keeps polling after the wait returned. A minute's timeout at a
	// millisecond's interval would be tens of thousands of requests if the
	// loop had carried on.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, settled, atomic.LoadInt32(&polls),
		"the wait went on calling the API after it returned")
}

func wantsBuild(r *http.Request) bool {
	return strings.Contains(r.URL.Path, "/builds/")
}

func wantsReplacement(r *http.Request) bool {
	return strings.Contains(r.URL.Path, "/replacement")
}
