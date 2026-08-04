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

package enclave

import (
	"encoding/json"
	"errors"
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

// installSkipAuth configures viper so drapi.AuthorizeRequest does not attempt
// to verify a token over the network.
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

// installEndpoint redirects config.GetEndpointURL to the given test server URL.
func installEndpoint(t *testing.T, url string) {
	t.Helper()

	prev := viperx.GetString(config.DataRobotURL)

	viperx.Set(config.DataRobotURL, url)

	t.Cleanup(func() {
		viperx.Set(config.DataRobotURL, prev)
	})
}

func TestParseLabels(t *testing.T) {
	t.Run("nil for no values", func(t *testing.T) {
		labels, err := ParseLabels(nil)
		require.NoError(t, err)
		assert.Nil(t, labels)
	})

	t.Run("parses key=value pairs", func(t *testing.T) {
		labels, err := ParseLabels([]string{"team=research", "env=prod"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"team": "research", "env": "prod"}, labels)
	})

	t.Run("keeps = in value", func(t *testing.T) {
		labels, err := ParseLabels([]string{"cfg=a=b"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"cfg": "a=b"}, labels)
	})

	t.Run("rejects missing =", func(t *testing.T) {
		_, err := ParseLabels([]string{"noequals"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})

	t.Run("rejects empty key", func(t *testing.T) {
		_, err := ParseLabels([]string{"=value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})
}

func TestRegisterEnclave(t *testing.T) {
	installSkipAuth(t)

	var gotBody registerRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts", r.URL.Path)

		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &gotBody))

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"outpostId": "abc-123",
			"name": "my-enclave",
			"status": "REGISTERED",
			"installationSecrets": {
				"drauth-service-registration-controller": {
					"data": {"hydraClientId": "cid", "hydraClientSecret": "csecret"}
				}
			}
		}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	result, err := RegisterEnclave("my-enclave", map[string]string{"team": "research"})
	require.NoError(t, err)

	assert.Equal(t, "my-enclave", gotBody.Name)
	assert.Equal(t, map[string]string{"team": "research"}, gotBody.Labels)

	assert.Equal(t, "abc-123", result.EnclaveID)
	assert.Equal(t, "REGISTERED", result.Status)
	require.Contains(t, result.InstallationSecrets, "drauth-service-registration-controller")
	assert.Equal(t, "cid",
		result.InstallationSecrets["drauth-service-registration-controller"].Data["hydraClientId"])
}

func TestRegisterEnclaveConflict(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail": "already exists"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := RegisterEnclave("dup", nil)
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func TestGetEnclave(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts/abc-123", r.URL.Path)

		_, _ = w.Write([]byte(`{
			"id": "abc-123",
			"name": "my-enclave",
			"dispatch_url": "https://enclave.example.com",
			"status": "ACTIVE"
		}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	e, err := GetEnclave("abc-123")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", e.ID)
	assert.Equal(t, "my-enclave", e.Name)
	assert.Equal(t, "https://enclave.example.com", e.DispatchURL)
	assert.Equal(t, "ACTIVE", e.Status)
}

func TestGetEnclaveEscapesID(t *testing.T) {
	installSkipAuth(t)

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		_, _ = w.Write([]byte(`{"id": "x", "name": "n", "status": "ACTIVE"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetEnclave("a/b?c")
	require.NoError(t, err)
	assert.Equal(t, "/covalent/api/v2/outposts/a%2Fb%3Fc", gotPath)
}

func TestListEnclaves(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts", r.URL.Path)

		_, _ = w.Write([]byte(`[
			{"id": "1", "name": "a", "dispatch_url": "", "status": "REGISTERED"},
			{"id": "2", "name": "b", "dispatch_url": "https://b", "status": "ACTIVE"}
		]`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	enclaves, err := ListEnclaves()
	require.NoError(t, err)
	require.Len(t, enclaves, 2)
	assert.Equal(t, "a", enclaves[0].Name)
	assert.Equal(t, "ACTIVE", enclaves[1].Status)
}

func TestDeactivateEnclave(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts/abc-123/deactivate", r.URL.Path)

		_, _ = w.Write([]byte(`{"id": "abc-123", "name": "n", "status": "DRAIN"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	e, err := DeactivateEnclave("abc-123")
	require.NoError(t, err)
	assert.Equal(t, "DRAIN", e.Status)
}

func TestReactivateEnclave(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts/abc-123/reactivate", r.URL.Path)

		_, _ = w.Write([]byte(`{"id": "abc-123", "name": "n", "status": "ACTIVE"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	e, err := ReactivateEnclave("abc-123")
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", e.Status)
}

func TestDeleteEnclave(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/covalent/api/v2/outposts/abc-123", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteEnclave("abc-123"))
}

func TestDeleteEnclaveNotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteEnclave("missing")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.True(t, errors.As(err, &httpErr))
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}
