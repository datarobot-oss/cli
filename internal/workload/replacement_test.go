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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTerminalReplacementStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"completed", true},
		{"failed", true},
		{"errored", true},
		{"submitted", false},
		{"initializing", false},
		{"candidate-warming", false},
		{"switching", false},
		{"", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsTerminalReplacementStatus(c.status), "status %q", c.status)
	}
}

func TestIsFailedReplacementStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"failed", true},
		{"errored", true},
		{"completed", false},
		{"submitted", false},
		{"", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsFailedReplacementStatus(c.status), "status %q", c.status)
	}
}

func TestGetActiveReplacement_Decodes(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-1/replacement/", r.URL.Path)
		fmt.Fprint(w, `{"id":"rep-1","workloadId":"wl-1","candidateArtifactId":"art-2","status":"submitted","strategy":"rolling"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := GetActiveReplacement("wl-1")
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, "art-2", replacement.ArtifactID)
	assert.Equal(t, "submitted", replacement.Status)
}

func TestGetActiveReplacement_404ReturnsNilNil(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := GetActiveReplacement("wl-1")
	require.NoError(t, err)
	assert.Nil(t, replacement)
}

func TestGetActiveReplacement_OtherErrorPropagates(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetActiveReplacement("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}

func TestStartReplacement_PostsArtifactIDAndRollingStrategy(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-1/replacement/", r.URL.Path)

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "art-2", body["artifactId"])
		assert.Equal(t, "rolling", body["strategy"])

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := StartReplacement("wl-1", "art-2")
	require.NoError(t, err)
	assert.Equal(t, "art-2", replacement.ArtifactID)
	assert.Equal(t, "submitted", replacement.Status)
}

func TestWaitForReplacement_TerminalCompletedReturnsNil(t *testing.T) {
	installSkipAuth(t)

	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := atomic.AddInt32(&hits, 1)

		status := "submitted"
		if page >= 2 {
			status = ReplacementStatusCompleted
		}

		fmt.Fprintf(w, `{"candidateArtifactId":"art-2","status":"%s"}`, status)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := WaitForReplacement("wl-1", time.Millisecond, time.Second)
	require.NoError(t, err)
	assert.Equal(t, ReplacementStatusCompleted, replacement.Status)
}

func TestWaitForReplacement_FailedReturnsError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"failed"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := WaitForReplacement("wl-1", time.Millisecond, time.Second)
	require.Error(t, err)
	require.NotNil(t, replacement, "failed returns the final replacement alongside the error")
	assert.Equal(t, "failed", replacement.Status)
	assert.Contains(t, err.Error(), "reverted")
}

// TestWaitForReplacement_ErroredClearedViaNotFound guards the exact live bug
// found against staging: a candidate can be marked "errored" (not one of the
// skill-documented "completed"/"failed" pair) and then have the platform
// clear the record with a 404 shortly after -- if "errored" weren't
// recognized as terminal-failure, the 404-after-seen branch would report
// this as a quiet success.
func TestWaitForReplacement_ErroredClearedViaNotFound(t *testing.T) {
	installSkipAuth(t)

	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := atomic.AddInt32(&hits, 1)

		if page == 1 {
			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"errored"}`)

			return
		}

		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := WaitForReplacement("wl-1", time.Millisecond, time.Second)
	require.Error(t, err, "an errored candidate must not be reported as success just because it was later cleared")
	require.NotNil(t, replacement)
	assert.Equal(t, ReplacementStatusErrored, replacement.Status)
}

func TestWaitForReplacement_NotFoundOnFirstPollIsError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := WaitForReplacement("wl-1", time.Millisecond, time.Second)
	require.Error(t, err)
	assert.Nil(t, replacement)
	assert.Contains(t, err.Error(), "no active replacement")
}

// TestWaitForReplacement_NonTerminalClearedViaNotFoundIsSuccess: the
// documented case wait_for_replacement.py handles -- the platform settles
// a replacement and garbage-collects the record before the next poll lands.
func TestWaitForReplacement_NonTerminalClearedViaNotFoundIsSuccess(t *testing.T) {
	installSkipAuth(t)

	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := atomic.AddInt32(&hits, 1)

		if page == 1 {
			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"switching"}`)

			return
		}

		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail":"There is no active replacement for this workload."}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	replacement, err := WaitForReplacement("wl-1", time.Millisecond, time.Second)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, "switching", replacement.Status)
}

func TestWaitForReplacement_Timeout(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := WaitForReplacement("wl-1", 5*time.Millisecond, 25*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}
