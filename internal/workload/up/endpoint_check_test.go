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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The status is stated, not judged: a 404 at / is how a healthy API-only
// framework answers, and only the app's author knows whether that is fine.
func TestVerifyEndpoint_StatesTheStatusInWords(t *testing.T) {
	force(t, &checkEndpointFn, func(url string) (int, error) {
		assert.Equal(t, "https://x.example/w/", url)

		return http.StatusNotFound, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"}, "", newReporter(&out, false))

	assert.Contains(t, out.String(), "404 Not Found")
	assert.Contains(t, out.String(), "anonymous GET")
}

// An endpoint that does not answer is the case this check exists for: with no
// probe written by default, "running" only means the container started, and
// this is the one line telling the user their green tick serves nothing yet.
func TestVerifyEndpoint_SaysPlainlyWhenNothingAnswers(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) {
		return 0, errors.New("dial tcp: connection refused")
	})

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"}, "", newReporter(&out, false))

	text := out.String()
	assert.Contains(t, text, "did not answer")
	assert.Contains(t, text, "connection refused")
	assert.Contains(t, text, "only means the container started")
	assert.Contains(t, text, "dr workload logs wl-1")
}

// No endpoint, nothing to check: a run that produced no URL has nothing whose
// silence could surprise anyone.
func TestVerifyEndpoint_SkipsWithoutAnEndpoint(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) {
		t.Fatal("nothing may be fetched when there is no endpoint")

		return 0, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{}, "", newReporter(&out, false))
	assert.Empty(t, out.String())
}

// The real GET is anonymous. The user's API token has no business in a
// container's access log, and a 401 proves something is serving as well as a
// 200 does.
func TestCheckEndpoint_SendsNoCredentials(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusTeapot)
	}))

	defer srv.Close()

	status, err := checkEndpointFn(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Empty(t, gotAuth, "the GET carries no credentials")
}

// A redirect is reported as the 3xx it is, never followed: a gateway that
// 302s an anonymous GET to a login page would otherwise have the check
// report the login page's 200 for a container serving nothing — the one
// case the check exists to catch.
func TestCheckEndpoint_ReportsTheRedirectItselfWithoutFollowing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			t.Fatal("the redirect must not be followed")
		}

		http.Redirect(w, r, "/login", http.StatusFound)
	}))

	defer srv.Close()

	status, err := checkEndpointFn(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, status)
}

// A deploy whose endpoint answers nothing still succeeds. Failing it would be
// the probe mistake again: a guess about the app's routes killing a deploy
// the platform considers healthy.
func TestRun_EndpointNotAnsweringDoesNotFailTheRun(t *testing.T) {
	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
		checkEndpoint: func(string) (int, error) { return 0, errors.New("timeout awaiting headers") },
	})

	result, stderr, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err, "the check reports; it must never fail the deploy")
	assert.Equal(t, "wl-new", result.WorkloadID)
	assert.Contains(t, stderr, "did not answer")
	assert.Contains(t, stderr, "dr workload logs wl-new")
}

// The happy path ends with the check's one line, so the transcript says what
// the endpoint actually did rather than leaving the green tick to imply it.
func TestRun_ReportsTheEndpointAnswer(t *testing.T) {
	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
		checkEndpoint: func(string) (int, error) { return http.StatusOK, nil },
	})

	_, stderr, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "Endpoint check")
	assert.Contains(t, stderr, "200 OK")
}
