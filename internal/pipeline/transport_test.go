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

package pipeline

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installSkipAuth configures viper so drapi.AuthorizeRequest does not
// attempt to verify a token over the network. It is safe to call from
// every test; previous values are restored at cleanup.
func installSkipAuth(t *testing.T) {
	t.Helper()

	prevSkip := viperx.GetBool("skip_auth")
	prevTok := viperx.GetString(config.DataRobotAPIKey)

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		viperx.Set("skip_auth", prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevTok)
	})
}

// installEndpoint temporarily sets the DataRobot URL viper key to url,
// restoring the previous value at test cleanup.
func installEndpoint(t *testing.T, url string) {
	t.Helper()

	prev := viperx.GetString(config.DataRobotURL)

	viperx.Set(config.DataRobotURL, url)

	t.Cleanup(func() {
		viperx.Set(config.DataRobotURL, prev)
	})
}

func TestBuildJSONRequest_BodyAndHeaders(t *testing.T) {
	installSkipAuth(t)

	req, err := buildJSONRequest(http.MethodPost, "http://example/x", map[string]string{"a": "b"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.NotEmpty(t, req.Header.Get("Authorization"))

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var parsed map[string]string

	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, "b", parsed["a"])
}

func TestBuildJSONRequest_NilBodyOmitsContentType(t *testing.T) {
	installSkipAuth(t)

	req, err := buildJSONRequest(http.MethodPatch, "http://example/x", nil)
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Content-Type"))
}

func TestDoJSON_DecodesSuccess(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]string

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "in-1", body["input_id"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))

	defer srv.Close()

	var out map[string]bool

	err := doJSON(http.MethodPost, srv.URL, map[string]string{"input_id": "in-1"}, "test", &out)
	require.NoError(t, err)
	assert.True(t, out["ok"])
}

func TestDoJSON_NilOutDiscardsResponse(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ignored": "value"}`))
	}))

	defer srv.Close()

	require.NoError(t, doJSON(http.MethodGet, srv.URL, nil, "", nil))
}

func TestDoJSON_404ReturnsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	var out map[string]any

	err := doJSON(http.MethodGet, srv.URL, nil, "", &out)
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestDoJSON_400WithDetailReturnsFormattedError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail": "lattice missing"}`))
	}))

	defer srv.Close()

	var out map[string]any

	err := doJSON(http.MethodPost, srv.URL, map[string]string{}, "", &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
	assert.Contains(t, err.Error(), "lattice missing")
}

func TestDoDelete_SuccessOn2xx(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.NotEmpty(t, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	require.NoError(t, doDelete(srv.URL, "test"))
}

func TestDoDelete_404ReturnsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	err := doDelete(srv.URL, "test")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestDoDelete_409WithDetailReturnsFormattedError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail": "already terminal"}`))
	}))

	defer srv.Close()

	err := doDelete(srv.URL, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 409")
	assert.Contains(t, err.Error(), "already terminal")
}
