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

package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/drapi"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useServer points the CLI's configured install at a test server. SkipAuthKey
// makes the token resolve from viper, so no verification request reaches the
// handler.
func useServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)

	viperx.Reset()
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	return srv
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestCall_SendsTheTokenOnAnAuthenticatedRoute(t *testing.T) {
	var got string

	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")

		writeJSON(w, http.StatusOK, `{}`)
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodGet, url, true, nil, &struct{}{}))
	assert.Equal(t, "Bearer test-token", got)
}

func TestCall_SendsNoTokenToAPublicRoute(t *testing.T) {
	var got string

	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")

		writeJSON(w, http.StatusOK, `{}`)
	})

	url, err := publicURL("/config")
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodGet, url, false, nil, &struct{}{}))
	assert.Empty(t, got)
}

func TestCall_KeepsTheErrorBodyAsTheReason(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, `{"message":"Missing seat"}`)
	})

	url, err := apiURL("/mlops/compute/bundles/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.Equal(t, `HTTP 403: {"message":"Missing seat"}`, reason(err))
}

func TestReason_WithoutABodyNamesTheStatus(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	assert.Equal(t, "HTTP 502", reason(err))
}

func TestCall_ReportsATransportFailure(t *testing.T) {
	srv := useServer(t, func(http.ResponseWriter, *http.Request) {})
	srv.Close()

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.True(t, isTransport(err))
}

type item struct {
	ID string `json:"id"`
}

func TestListAll_FollowsNextAcrossPages(t *testing.T) {
	var srv *httptest.Server

	srv = useServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeJSON(w, http.StatusOK, `{"data":[{"id":"c"}],"next":null}`)

			return
		}

		writeJSON(w, http.StatusOK, fmt.Sprintf(`{"data":[{"id":"a"},{"id":"b"}],"next":"%s/api/v2/things/?page=2"}`, srv.URL))
	})

	url, err := apiURL("/things/", nil)
	require.NoError(t, err)

	got, err := listAll[item](context.Background(), url)

	require.NoError(t, err)
	assert.Equal(t, []item{{"a"}, {"b"}, {"c"}}, got)
}

func TestListAll_RefusesANextCursorOnAnotherHost(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"data":[{"id":"a"}],"next":"https://elsewhere.example/api/v2/things/?page=2"}`)
	})

	url, err := apiURL("/things/", nil)
	require.NoError(t, err)

	_, err = listAll[item](context.Background(), url)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match API base")
}

func TestListAll_ReturnsTheErrorOfAFailingPage(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, `{"message":"no"}`)
	})

	url, err := apiURL("/things/", nil)
	require.NoError(t, err)

	_, err = listAll[item](context.Background(), url)

	assert.Equal(t, `HTTP 403: {"message":"no"}`, reason(err))
}

// pagedServer answers every request with cursor(n), where n counts the
// requests so far starting at 1, and reports how many requests it saw.
func pagedServer(t *testing.T, cursor func(base string, n int) string, items int) *atomic.Int64 {
	t.Helper()

	var (
		seen atomic.Int64
		srv  *httptest.Server
	)

	srv = useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		n := int(seen.Add(1))
		rows := strings.TrimSuffix(strings.Repeat(`{"id":"x"},`, items), ",")

		writeJSON(w, http.StatusOK, fmt.Sprintf(`{"data":[%s],"next":%q}`, rows, cursor(srv.URL, n)))
	})

	return &seen
}

func listThings(t *testing.T) error {
	t.Helper()

	url, err := apiURL("/things/", nil)
	require.NoError(t, err)

	_, err = listAll[item](context.Background(), url)

	return err
}

func TestListAll_StopsAtACursorThatPointsBackAtAPageAlreadyRead(t *testing.T) {
	seen := pagedServer(t, func(base string, _ int) string { return base + "/api/v2/things/" }, 1)

	err := listThings(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "repeats a page already read")
	assert.LessOrEqual(t, seen.Load(), int64(2))
}

func TestListAll_StopsAfterTheLastAllowedPage(t *testing.T) {
	seen := pagedServer(t, func(base string, n int) string { return fmt.Sprintf("%s/api/v2/things/?page=%d", base, n) }, 1)

	err := listThings(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("more than %d pages", maxPages))
	assert.Equal(t, int64(maxPages), seen.Load())
}

func TestListAll_StopsAfterTheLastAllowedItem(t *testing.T) {
	pagedServer(t, func(base string, n int) string { return fmt.Sprintf("%s/api/v2/things/?page=%d", base, n) }, maxItems/4)

	err := listThings(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("more than %d items", maxItems))
}

func TestListAll_ReadsAListThatEndsWithinTheCaps(t *testing.T) {
	pagedServer(t, func(base string, n int) string {
		if n >= maxPages {
			return ""
		}

		return fmt.Sprintf("%s/api/v2/things/?page=%d", base, n)
	}, 1)

	require.NoError(t, listThings(t))
}

func TestListAll_RefusesACursorOnAnotherPathOfTheSameHost(t *testing.T) {
	pagedServer(t, func(base string, _ int) string { return base + "/api/v2/other/?x=1" }, 1)

	err := listThings(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "leaves the route")
}

func TestListAll_RefusesACursorThatCarriesUserInfo(t *testing.T) {
	pagedServer(t, func(base string, _ int) string {
		return strings.Replace(base, "http://", "http://user:pw@", 1) + "/api/v2/things/?page=2"
	}, 1)

	err := listThings(t)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user information")
}

func TestCall_RefusesAResponseLargerThanTheCap(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"pad":"`+strings.Repeat("a", maxBodyBytes)+`"}`)
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than")
	assert.True(t, isTransport(err), "an answer no DataRobot route gives reads as an unusable answer")
}

func TestCall_ReadsAResponseJustWithinTheCap(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"pad":"`+strings.Repeat("a", maxBodyBytes-20)+`"}`)
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodGet, url, true, nil, &struct{}{}))
}

// A redirect may not carry the token to another scheme, port or host of the
// install, though Go itself strips it only for another hostname.
func TestCall_NeverSendsTheTokenToAnotherOriginThroughARedirect(t *testing.T) {
	targets := map[string]func() *httptest.Server{
		"another port":   func() *httptest.Server { return httptest.NewServer(nil) },
		"another scheme": func() *httptest.Server { return httptest.NewTLSServer(nil) },
	}

	for name, start := range targets {
		t.Run(name, func(t *testing.T) {
			var leaked atomic.Int64

			target := start()
			target.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					leaked.Add(1)
				}

				writeJSON(w, http.StatusOK, `{}`)
			})

			t.Cleanup(target.Close)

			useServer(t, func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound) //nolint:gosec // the test server redirects on purpose
			})

			url, err := apiURL("/version/", nil)
			require.NoError(t, err)

			err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "redirect")
			assert.Zero(t, leaked.Load(), "the target received the bearer token")
		})
	}
}

func TestCall_NeverSendsTheTokenToAnotherHostnameThroughARedirect(t *testing.T) {
	var reached atomic.Int64

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		writeJSON(w, http.StatusOK, `{}`)
	}))
	t.Cleanup(target.Close)

	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(target.URL, "127.0.0.1", "localhost", 1)+r.URL.Path, http.StatusFound) //nolint:gosec // the test server redirects on purpose
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.Zero(t, reached.Load())
}

func TestCall_FollowsARedirectWithinTheSameOrigin(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/moved/" {
			writeJSON(w, http.StatusOK, `{}`)

			return
		}

		http.Redirect(w, r, "/api/v2/moved/", http.StatusFound)
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodGet, url, true, nil, &struct{}{}))
}

func TestReason_NeverShowsTheTokenAnErrorPageEchoes(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadGateway, "upstream said: Authorization: Bearer test-token and again test-token, plus bearer  other-secret")
	})

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.NotContains(t, reason(err), "test-token")
	assert.NotContains(t, reason(err), "other-secret")
	assert.Contains(t, reason(err), "HTTP 502: upstream said: Authorization: Bearer [redacted] and again [redacted]")
	assert.NotContains(t, err.Error(), "test-token", "the cause carries no token either")
}

func TestCall_LogsTheRequestWithoutTheToken(t *testing.T) {
	var logged bytes.Buffer

	prevLevel := log.GetLevel()

	log.SetOutput(&logged)
	log.SetLevel(log.DebugLevel)

	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(prevLevel)
	})

	useServer(t, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, `{}`) })

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodGet, url, true, nil, &struct{}{}))

	assert.Contains(t, logged.String(), "Request Info")
	assert.Contains(t, logged.String(), "/api/v2/version/")
	assert.NotContains(t, logged.String(), "test-token")
}

func TestIsTransport_IsFalseForAFailureThatNeverLeftTheMachine(t *testing.T) {
	viperx.Set(config.DataRobotURL, "")

	_, err := apiURL("/version/", nil)

	require.Error(t, err)
	assert.False(t, isTransport(err))
	assert.Zero(t, statusOf(err))
}

// useBackoff replaces the wait between attempts with a recorder, and the jitter
// with its upper bound, so a test neither sleeps nor depends on chance.
func useBackoff(t *testing.T) *[]time.Duration {
	t.Helper()

	var waits []time.Duration

	prevSleep, prevJitter := sleep, jitter

	sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)

		return nil
	}
	jitter = func(upper time.Duration) time.Duration { return upper }

	t.Cleanup(func() { sleep, jitter = prevSleep, prevJitter })

	return &waits
}

// flaky answers the first failures requests with status and header, then 200.
func flaky(t *testing.T, failures int, status int, retryAfter string) (*atomic.Int64, *[]string) {
	t.Helper()

	var (
		seen   atomic.Int64
		bodies []string
	)

	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))

		if int(seen.Add(1)) <= failures {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}

			writeJSON(w, status, `{"message":"slow down"}`)

			return
		}

		writeJSON(w, http.StatusOK, `{}`)
	})

	return &seen, &bodies
}

func getVersion(t *testing.T) error {
	t.Helper()

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	return call(context.Background(), http.MethodGet, url, true, nil, &struct{}{})
}

func TestCall_RetriesATooManyRequestsAnswerAfterTheWaitTheServerAsks(t *testing.T) {
	waits := useBackoff(t)
	seen, _ := flaky(t, 1, http.StatusTooManyRequests, "2")

	require.NoError(t, getVersion(t))

	assert.Equal(t, int64(2), seen.Load())
	assert.Equal(t, []time.Duration{2 * time.Second}, *waits)
}

func TestCall_RetriesAServiceUnavailableAnswerWithGrowingJitteredWaits(t *testing.T) {
	waits := useBackoff(t)
	seen, _ := flaky(t, 2, http.StatusServiceUnavailable, "")

	require.NoError(t, getVersion(t))

	assert.Equal(t, int64(3), seen.Load())
	assert.Equal(t, []time.Duration{retryBase, 2 * retryBase}, *waits)
}

func TestCall_GivesUpAfterTheLastAllowedRetryAndReportsTheStatus(t *testing.T) {
	useBackoff(t)
	seen, _ := flaky(t, 100, http.StatusServiceUnavailable, "")

	err := getVersion(t)

	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, statusOf(err))
	assert.Equal(t, int64(1+maxRetries), seen.Load())
}

func TestCall_DoesNotRetryWhenTheServerAsksForALongerWaitThanTheCap(t *testing.T) {
	waits := useBackoff(t)
	seen, _ := flaky(t, 100, http.StatusTooManyRequests, "120")

	err := getVersion(t)

	assert.Equal(t, http.StatusTooManyRequests, statusOf(err))
	assert.Equal(t, int64(1), seen.Load())
	assert.Empty(t, *waits)
}

func TestCall_ReadsARetryAfterGivenAsADate(t *testing.T) {
	waits := useBackoff(t)

	prev := now
	now = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

	t.Cleanup(func() { now = prev })

	seen, _ := flaky(t, 1, http.StatusTooManyRequests, "Tue, 24 Nov 2026 12:00:03 GMT")

	require.NoError(t, getVersion(t))

	assert.Equal(t, int64(2), seen.Load())
	assert.Equal(t, []time.Duration{3 * time.Second}, *waits)
}

func TestCall_DoesNotRetryAnAnswerThatARetryCannotChange(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			useBackoff(t)
			seen, _ := flaky(t, 100, status, "1")

			err := getVersion(t)

			assert.Equal(t, status, statusOf(err))
			assert.Equal(t, int64(1), seen.Load())
		})
	}
}

func TestCall_SendsTheSameBodyOnARetry(t *testing.T) {
	useBackoff(t)
	_, bodies := flaky(t, 1, http.StatusServiceUnavailable, "")

	url, err := apiURL("/entitlements/evaluate/", nil)
	require.NoError(t, err)

	require.NoError(t, call(context.Background(), http.MethodPost, url, true, map[string]string{"k": "v"}, &struct{}{}))

	assert.Equal(t, []string{`{"k":"v"}`, `{"k":"v"}`}, *bodies)
}

func TestCall_StopsRetryingWhenTheRunIsCancelled(t *testing.T) {
	prev := sleep
	sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	t.Cleanup(func() { sleep = prev })

	seen, _ := flaky(t, 100, http.StatusServiceUnavailable, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	url, err := apiURL("/version/", nil)
	require.NoError(t, err)

	err = call(ctx, http.MethodGet, url, true, nil, &struct{}{})

	require.Error(t, err)
	assert.LessOrEqual(t, seen.Load(), int64(1))
}

func TestCall_CarriesTheCleanedErrorBodyAsTheDetailOfTheHTTPError(t *testing.T) {
	useServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, "no seat\x1b[2J for \u202eyou")
	})

	err := getVersion(t)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, "no seat[2J for you", httpErr.Detail)
	assert.NotContains(t, err.Error(), "\x1b")
}

func TestReason_CleansAnErrorThatIsNotHTTP(t *testing.T) {
	err := errors.New("x509: certificate is valid for evil\x1b]0;pwn\x07.example.com, not dr.example.com")

	assert.Equal(t, "x509: certificate is valid for evil]0;pwn.example.com, not dr.example.com", reason(err))
}
