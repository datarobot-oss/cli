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

package rollout

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/cmd/internal/pollflags"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installTestServer wires viperx so config.GetEndpointURL resolves against
// srv, mirroring the pattern used by cmd/llm-gateway/list's cmd_test.go.
func installTestServer(t *testing.T, srv *httptest.Server) {
	t.Helper()

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotURL, srv.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

func testCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	return cmd
}

func TestApply_Stage_SkipsConfirmationAndNetwork(t *testing.T) {
	// No server installed at all: any network call would fail loudly (no
	// endpoint configured), so a passing test proves --stage never reaches
	// the network.
	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", true, Options{Stage: true})
	require.NoError(t, err)
	assert.Contains(t, cmd.OutOrStdout().(*bytes.Buffer).String(), "Staged artifact art-2")
}

func TestApply_RequiresYesWhenNonInteractive(t *testing.T) {
	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Contains(t, err.Error(), "--yes")
}

func TestApply_GuardBlocksWhenReplacementInFlight(t *testing.T) {
	var startCalled bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			startCalled = true
		}

		fmt.Fprint(w, `{"candidateArtifactId":"art-1","status":"submitted"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{Yes: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a replacement in progress")
	assert.False(t, startCalled, "must not start a second replacement while one is in flight")
}

// TestApply_StartReplacementFailureNamesThePreparedArtifact guards against
// silently losing track of a prepared artifact: if StartReplacement fails,
// targetArtifactID was already created/locked and the edit already landed
// on it -- the error must name it so the caller can retry the rollout
// instead of redoing the whole env edit.
func TestApply_StartReplacementFailureNamesThePreparedArtifact(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)

			return
		}

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"detail":"Artifact status mismatch"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{Yes: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-2", "the prepared artifact's id must be named in the error")
	assert.Contains(t, err.Error(), "wl-1")
}

func TestApply_LocksBeforeStartingReplacementWhenNeedsLock(t *testing.T) {
	var order []string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/artifacts/art-2/", func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "lock")

		fmt.Fprint(w, `{"id":"art-2","status":"locked"}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)

			return
		}

		order = append(order, "start")

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", true, Options{Yes: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"lock", "start"}, order, "the clone must be locked before the replacement is started")
}

func TestApply_StartsReplacementWithoutWaitingByDefault(t *testing.T) {
	var startCalled bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)

			return
		}

		startCalled = true

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{Yes: true})
	require.NoError(t, err)
	assert.True(t, startCalled)
	assert.Contains(t, cmd.ErrOrStderr().(*bytes.Buffer).String(), "Replacement started")
}

func TestApply_WaitReturnsSuccessOnCompleted(t *testing.T) {
	var (
		started  bool
		pollHits int
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			started = true

			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)

			return
		}

		// GET: 404 for the pre-flight guard (nothing in flight yet), then
		// one non-terminal poll before settling to "completed" once
		// WaitForReplacement starts polling after start.
		if !started {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)

			return
		}

		pollHits++

		if pollHits == 1 {
			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)

			return
		}

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"completed"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	poll := pollflags.Set{Wait: true, Interval: 0, Timeout: 0}
	// pollflags.Set's zero Interval/Timeout would hot-loop or never elapse in
	// production use, but Register's positiveDurationValue guard only fires
	// at flag-parse time -- direct construction here just needs a tiny but
	// nonzero pair so WaitForReplacement's deadline math behaves.
	poll.Interval = 1
	poll.Timeout = 1_000_000_000 // 1s in nanoseconds, avoids importing "time" just for this

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{Yes: true, Poll: poll})
	require.NoError(t, err, "a completed replacement must not be reported as an error")
}

func TestApply_WaitReturnsErrorOnFailed(t *testing.T) {
	var started bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/replacement/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			started = true

			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)

			return
		}

		// GET: 404 for the pre-flight guard (nothing in flight yet), then
		// "failed" once WaitForReplacement starts polling after start.
		if !started {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)

			return
		}

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"failed"}`)
	})

	installTestServer(t, httptest.NewServer(mux))

	cmd := testCmd()

	poll := pollflags.Set{Wait: true, Interval: 1, Timeout: 1_000_000_000}

	err := Apply(cmd, outputformat.OutputFormatText, "wl-1", "art-2", false, Options{Yes: true, Poll: poll})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reverted")
}
