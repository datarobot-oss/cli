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
	"net/http"
	"net/http/httptest"
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

	verifyEndpoint(Result{Endpoint: "https://x.example/w/", WorkloadID: "wl-1"}, "", newReporter(&out, false))

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
	}, "", newReporter(&out, false))

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
	}, "", newReporter(&out, false))

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

// The check asks the route the served artifact's readiness probe names, which
// is the one the platform's readiness gate asks before it reports running, and
// asks / only when there is none.
func TestTargetFor(t *testing.T) {
	const (
		root   = "https://app.example.test/api/v2/endpoints/workloads/wl-1/"
		pinned = "https://app.example.test/api/v2/endpoints/workloads/wl-1?protonId=p-1"
	)

	probe := func(member string) workload.Document {
		return servedDoc(t, `, "readinessProbe": `+member)
	}

	sidecarFirst := doc(t, `{"spec": {"containerGroups": [{"containers": [
		{"name": "bridge", "readinessProbe": {"path": "/bridge"}},
		{"name": "primary", "primary": true, "port": 8080, "readinessProbe": {"path": "/version"}}
	]}]}}`)

	tests := []struct {
		name      string
		endpoint  string
		served    workload.Document
		wantURL   string
		wantProbe string
	}{
		{"no artifact read", root, nil, root, ""},
		{"no readiness probe", root, servedDoc(t, ""), root, ""},
		{"blank probe path", root, probe(`{"path": "  ", "port": 8080}`), root, ""},
		{"probe path", root, probe(`{"path": "/version", "port": 8080}`), root + "version", "/version"},
		// The gate asks the path on the container's port whatever the probe
		// names, so a workload that reached running answers it there.
		{"probe on another port", root, probe(`{"path": "/version", "port": 9000}`), root + "version", "/version"},
		{"path without a leading slash", root, probe(`{"path": "health"}`), root + "health", "health"},
		{"trailing slash kept", root, probe(`{"path": "/health/"}`), root + "health/", "/health/"},
		{
			"pinned endpoint keeps its query", pinned, probe(`{"path": "/version"}`),
			"https://app.example.test/api/v2/endpoints/workloads/wl-1/version?protonId=p-1", "/version",
		},
		{
			"probe query joins the endpoint's", pinned, probe(`{"path": "/health?full=1"}`),
			"https://app.example.test/api/v2/endpoints/workloads/wl-1/health?protonId=p-1&full=1", "/health?full=1",
		},
		{"the primary container's probe, not a sidecar's", root, sidecarFirst, root + "version", "/version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotProbe := targetFor(tt.endpoint, tt.served)

			assert.Equal(t, tt.wantURL, gotURL)
			assert.Equal(t, tt.wantProbe, gotProbe)
		})
	}
}

// The line names the path it asked, and qualifies a 404 only where the check
// fell back to /, which an app need not serve. A 404 on the probe's own path
// is a real finding and stays bare.
func TestVerifyEndpoint_NamesThePathItAsked(t *testing.T) {
	const endpoint = "https://x.example/w/"

	probed := servedDoc(t, `, "readinessProbe": {"path": "/version", "port": 8080}`)
	bare := servedDoc(t, "")

	tests := []struct {
		name    string
		served  workload.Document
		readErr error
		status  int
		wantURL string
		want    string
		hint    bool
	}{
		{
			"probe path answering", probed, nil, http.StatusOK, endpoint + "version",
			"anonymous GET of /version, the readiness probe's path, answered HTTP 200 OK.", false,
		},
		{
			"probe path missing", probed, nil, http.StatusNotFound, endpoint + "version",
			"GET of /version, the readiness probe's path, answered HTTP 404 Not Found.", false,
		},
		{"root answering", bare, nil, http.StatusOK, endpoint, "GET of / answered HTTP 200 OK.", false},
		{"root missing", bare, nil, http.StatusNotFound, endpoint, "GET of / answered HTTP 404 Not Found.", true},
		{
			"unreadable artifact", nil, errors.New("403 Forbidden"), http.StatusNotFound, endpoint,
			"GET of / answered HTTP 404 Not Found.", true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			force(t, &servedArtifactFn, func(id string) (workload.Document, error) {
				assert.Equal(t, "art-1", id)

				return tt.served, tt.readErr
			})
			force(t, &checkEndpointFn, func(url string) (int, error) {
				assert.Equal(t, tt.wantURL, url)

				return tt.status, nil
			})

			var out bytes.Buffer

			verifyEndpoint(Result{Endpoint: endpoint, WorkloadID: "wl-1", ArtifactID: "art-1"}, "", newReporter(&out, false))

			assert.Contains(t, out.String(), tt.want)

			if tt.hint {
				assert.Contains(t, out.String(), "serves nothing at /")
			} else {
				assert.NotContains(t, out.String(), "serves nothing at /")
			}
		})
	}
}

// A deploy whose endpoint answers nothing still succeeds. Failing it would be
// the probe mistake again: a guess about the app's routes killing a deploy
// the platform considers healthy.
func TestRun_EndpointNotAnsweringDoesNotFailTheRun(t *testing.T) {
	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
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
		wait: func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
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

// The probe is read off the artifact the workload ended up serving, not off
// the file. A manifest that names its artifact by id carries no probe at all,
// and its check still asks the route that artifact declares.
func TestRun_EndpointCheckAsksTheServedArtifactsProbe(t *testing.T) {
	var asked string

	install(t, fakes{
		create:  func(any) (*workload.Workload, error) { return running("wl-new"), nil },
		writeID: func(string, string) error { return nil },
		wait: func(context.Context, string, workload.Serving, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked}, nil
		},
		checkEndpoint: func(url string) (int, error) {
			asked = url

			return http.StatusOK, nil
		},
	})
	force(t, &servedArtifactFn, func(id string) (workload.Document, error) {
		assert.Equal(t, "art-1", id, "the artifact the wait saw serving")

		return doc(t, liveArtifactJSON), nil
	})

	_, stderr, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Equal(t, "https://app.datarobot.com/workloads/wl-new/health", asked)
	assert.Contains(t, stderr, "GET of /health, the readiness probe's path")
}

// servedDoc is an artifact as the server returns it, with one primary
// container on 8080. member is spliced into that container, for example
// `, "readinessProbe": {"path": "/version"}`; "" leaves it without a probe.
func servedDoc(t *testing.T, member string) workload.Document {
	t.Helper()

	return doc(t, `{"spec": {"containerGroups": [{"containers": [`+
		`{"name": "primary", "primary": true, "port": 8080`+member+`}]}]}}`)
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
