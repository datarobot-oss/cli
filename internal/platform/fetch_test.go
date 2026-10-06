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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
