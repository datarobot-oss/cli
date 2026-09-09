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
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	drlog "github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/testutil"
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

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"}, endpointCheck{}, newReporter(&out, false))

	assert.Contains(t, out.String(), "404 Not Found")
	assert.Contains(t, out.String(), "anonymous GET")
}

// Gateway-routed workload URLs are DataRobot API URLs: the gateway consumes the
// token before proxying, so the status can describe the workload instead of the
// gateway's unauthenticated 401.
func TestVerifyEndpoint_StatesAuthenticatedGatewayChecks(t *testing.T) {
	installEndpointCheckAuth(t, "https://app.example.test", "endpoint-test-token")
	force(t, &checkEndpointFn, func(url string) (int, error) {
		assert.Equal(t, "https://app.example.test/api/v2/endpoints/workloads/wl-1?protonId=p-1", url)

		return http.StatusOK, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{
		Endpoint:   "https://app.example.test/api/v2/endpoints/workloads/wl-1?protonId=p-1",
		WorkloadID: "wl-1",
	}, endpointCheck{}, newReporter(&out, false))

	assert.Contains(t, out.String(), "authenticated GET")
	assert.Contains(t, out.String(), "200 OK")
}

// An endpoint that does not answer is the case this check exists for: with no
// probe written by default, "running" only means the container started, and
// this is the one line telling the user their green tick serves nothing yet.
func TestVerifyEndpoint_SaysPlainlyWhenNothingAnswers(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) {
		return 0, errors.New("dial tcp: connection refused")
	})

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"}, endpointCheck{}, newReporter(&out, false))

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

	verifyEndpoint(Result{}, endpointCheck{}, newReporter(&out, false))
	assert.Empty(t, out.String())
}

// A malformed endpoint is not evidence that the workload failed to answer: the
// CLI never made a network request, so the report says the check was skipped.
func TestVerifyEndpoint_SkipsMalformedEndpoint(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) {
		t.Fatal("nothing may be fetched when the endpoint is not an absolute URL")

		return 0, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{
		Endpoint:   "None/api/v2/endpoints/workloads/wl-1?protonId=p-1",
		WorkloadID: "wl-1",
	}, endpointCheck{}, newReporter(&out, false))

	text := out.String()
	assert.Contains(t, text, "Skipping endpoint check")
	assert.Contains(t, text, "not absolute")
	assert.NotContains(t, text, "did not answer")
}

// A direct workload URL stays anonymous. The user's API token has no business
// in a container's access log, and a 401 proves something is serving as well as
// a 200 does when the request actually reaches the container.
func TestCheckEndpoint_SendsNoCredentialsToDirectEndpoint(t *testing.T) {
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

// The public workload endpoint is an API gateway URL. The CLI token is safe to
// attach there because the gateway consumes it before proxying to the container.
func TestCheckEndpoint_SendsCredentialsToConfiguredWorkloadGateway(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusTeapot)
	}))

	defer srv.Close()

	installEndpointCheckAuth(t, srv.URL, "endpoint-test-token")

	status, err := checkEndpointFn(srv.URL + "/api/v2/endpoints/workloads/wl-1?protonId=p-1")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, "Bearer endpoint-test-token", gotAuth)
}

// A gateway-shaped URL on the wrong origin stays anonymous for credential
// safety, but the fallback must leave a debug breadcrumb so a same-looking 401
// can be diagnosed as a token intentionally withheld by the CLI.
func TestEndpointCheckAuthForURL_LogsWhenGatewayOriginDoesNotMatchConfig(t *testing.T) {
	installEndpointCheckAuth(t, "https://configured.example.test", "endpoint-test-token")

	logDir := startEndpointCheckDebugLog(t)

	mode, err := endpointCheckAuthForURL("https://returned.example.test/api/v2/endpoints/workloads/wl-1")
	require.NoError(t, err)
	assert.Equal(t, endpointCheckAnonymous, mode)

	content, err := os.ReadFile(filepath.Join(logDir, ".dr-tui-debug.log"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "withholding token")
	assert.Contains(t, string(content), "returned.example.test")
	assert.Contains(t, string(content), "configured.example.test")
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

func installEndpointCheckAuth(t *testing.T, baseURL, token string) {
	t.Helper()

	prevBase := viperx.GetString(config.DataRobotURL)
	prevSkip := viperx.GetBool(config.SkipAuthKey)
	prevToken := viperx.GetString(config.DataRobotAPIKey)
	prevCachedToken := drapi.GetToken()

	viperx.Set(config.DataRobotURL, baseURL)
	viperx.Set(config.SkipAuthKey, true)
	viperx.Set(config.DataRobotAPIKey, token)
	drapi.SetToken("")

	t.Cleanup(func() {
		viperx.Set(config.DataRobotURL, prevBase)
		viperx.Set(config.SkipAuthKey, prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevToken)
		drapi.SetToken(prevCachedToken)
	})
}

func startEndpointCheckDebugLog(t *testing.T) string {
	t.Helper()

	prevDebug := viperx.GetBool("debug")
	logDir := t.TempDir()

	testutil.SetTestHomeDir(t, logDir)
	viperx.Set("debug", true)
	drlog.Start()

	t.Cleanup(func() {
		drlog.Stop()
		viperx.Set("debug", prevDebug)
	})

	return logDir
}

// TestMain drops the endpoint check's retry pause. The pause is there so a
// container that is still starting is not reported as serving nothing; nothing
// in this package is a real container, so sitting through it would only add
// seconds to every test that fakes a refused GET.
func TestMain(m *testing.M) {
	endpointCheckRetryDelay = 0

	os.Exit(m.Run())
}

// The point of the pin. A deploy now returns when the platform promotes the new
// generation, and for minutes after that the endpoint URL hands a share of
// requests to the version being replaced, so an unpinned GET is not a question
// about what this deploy put there.
func TestVerifyEndpoint_PinsTheGETToTheGenerationTheWaitSettledOn(t *testing.T) {
	installEndpointCheckAuth(t, "https://app.example.test", "endpoint-test-token")

	var got string

	force(t, &checkEndpointFn, func(url string) (int, error) {
		got = url

		return http.StatusOK, nil
	})

	var out bytes.Buffer

	result := Result{
		Endpoint:   "https://app.example.test/api/v2/endpoints/workloads/wl-1/",
		WorkloadID: "wl-1",
	}

	verifyEndpoint(result, endpointCheck{ProtonID: "p-9"}, newReporter(&out, false))

	assert.Equal(t, "https://app.example.test/api/v2/endpoints/workloads/wl-1/?protonId=p-9", got)
	assert.Contains(t, out.String(), "pinned to the container generation")
	assert.Equal(t, "https://app.example.test/api/v2/endpoints/workloads/wl-1/", result.Endpoint,
		"the pin belongs to the GET the CLI makes for itself; a pinned URL in the output would be a link "+
			"that dies when that generation is collected")
}

// An endpoint URL is allowed to carry a query of its own, so the parameter is
// merged rather than appended.
func TestVerifyEndpoint_PinMergesIntoAnExistingQuery(t *testing.T) {
	installEndpointCheckAuth(t, "https://app.example.test", "endpoint-test-token")

	var got string

	force(t, &checkEndpointFn, func(url string) (int, error) {
		got = url

		return http.StatusOK, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{
		Endpoint:   "https://app.example.test/api/v2/endpoints/workloads/wl-1/?tab=logs&protonId=stale",
		WorkloadID: "wl-1",
	}, endpointCheck{ProtonID: "p-9"}, newReporter(&out, false))

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	assert.Equal(t, "p-9", parsed.Query().Get("protonId"))
	assert.Equal(t, "logs", parsed.Query().Get("tab"), "the endpoint's own query survives the pin")
}

// protonId is the DataRobot gateway's parameter. On a direct service URL it
// would pin nothing and would arrive in somebody's application as a query
// parameter it never asked for.
func TestVerifyEndpoint_DoesNotPinADirectEndpoint(t *testing.T) {
	var got string

	force(t, &checkEndpointFn, func(url string) (int, error) {
		got = url

		return http.StatusOK, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"},
		endpointCheck{ProtonID: "p-9"}, newReporter(&out, false))

	assert.Equal(t, "https://x.example/w/", got)
	assert.NotContains(t, out.String(), "pinned")
}

// A workload gateway path on an origin that is not the configured one is not
// this gateway, and is exactly the case that already withholds the token.
func TestVerifyEndpoint_DoesNotPinAGatewayPathOnAnotherOrigin(t *testing.T) {
	installEndpointCheckAuth(t, "https://app.example.test", "endpoint-test-token")

	var got string

	force(t, &checkEndpointFn, func(url string) (int, error) {
		got = url

		return http.StatusOK, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{
		Endpoint:   "https://other.example.test/api/v2/endpoints/workloads/wl-1/",
		WorkloadID: "wl-1",
	}, endpointCheck{ProtonID: "p-9"}, newReporter(&out, false))

	assert.Equal(t, "https://other.example.test/api/v2/endpoints/workloads/wl-1/", got)
	assert.Contains(t, out.String(), "anonymous GET")
}

// The line that keeps the success report honest: the deploy returns before the
// platform has finished moving the route, and saying nothing would let the
// green tick imply the cutover is complete.
func TestVerifyEndpoint_SaysThePreviousVersionMayStillAnswer(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) { return http.StatusOK, nil })

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"},
		endpointCheck{PredecessorMayServe: true}, newReporter(&out, false))

	text := out.String()
	assert.Contains(t, text, "may still answer some requests")
	assert.Contains(t, text, "--wait-for-drain")
}

// It is a fact about the deploy, not about the GET, so it survives a run with
// no endpoint to check.
func TestVerifyEndpoint_SaysThePreviousVersionMayServeWithNoEndpointToCheck(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) {
		t.Fatal("nothing may be fetched when there is no endpoint")

		return 0, nil
	})

	var out bytes.Buffer

	verifyEndpoint(Result{WorkloadID: "wl-1"}, endpointCheck{PredecessorMayServe: true},
		newReporter(&out, false))

	assert.Contains(t, out.String(), "may still answer some requests")
}

// A run that waited the drain out has nothing to warn about: nothing else is
// serving, which is the whole of what it spent those minutes buying.
func TestVerifyEndpoint_SaysNothingAboutThePredecessorAfterADrainWait(t *testing.T) {
	force(t, &checkEndpointFn, func(string) (int, error) { return http.StatusOK, nil })

	var out bytes.Buffer

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"},
		endpointCheck{}, newReporter(&out, false))

	assert.NotContains(t, out.String(), "may still answer some requests")
}

// The GET now happens at the promotion rather than seven minutes after it, and
// a promotion says only that the container started, so a refusal to answer is
// given a few seconds to become an answer.
func TestCheckEndpointWithRetry_GivesAContainerTimeToStartAnswering(t *testing.T) {
	calls := 0

	force(t, &checkEndpointFn, func(string) (int, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("dial tcp: connection refused")
		}

		return http.StatusOK, nil
	})

	status, err := checkEndpointWithRetry("https://x.example/w/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, 2, calls)
}

// A status code is the application's answer, whatever it is: a 404 at / is how
// a healthy API-only framework answers, and asking again would only delay
// saying so.
func TestCheckEndpointWithRetry_DoesNotRetryTheApplicationsOwnAnswer(t *testing.T) {
	calls := 0

	force(t, &checkEndpointFn, func(string) (int, error) {
		calls++

		return http.StatusNotFound, nil
	})

	status, err := checkEndpointWithRetry("https://x.example/w/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, 1, calls)
}

// The three the gateway returns for itself when there is nothing behind it yet
// are not the application's answer either.
func TestCheckEndpointWithRetry_RetriesAGatewayWithNothingBehindIt(t *testing.T) {
	calls := 0

	force(t, &checkEndpointFn, func(string) (int, error) {
		calls++

		return http.StatusServiceUnavailable, nil
	})

	status, err := checkEndpointWithRetry("https://x.example/w/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, endpointCheckAttempts, calls, "it gives up and reports what it kept getting")
}
