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

package drapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetTokenForTest seeds the package-level token so Post() does not call
// config.GetAPIKey() (which would require a configured environment).
// Returns a cleanup function the test should defer.
func resetTokenForTest(t *testing.T, value string) func() {
	t.Helper()

	previous := token
	token = value

	return func() { token = previous }
}

func TestPost_Created(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer server.Close()

	resp, err := Post(server.URL, "", map[string]string{"name": "v"})
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestPost_OK(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	resp, err := Post(server.URL, "", map[string]string{})
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPost_NonSuccess(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	resp, err := Post(server.URL, "", map[string]string{}) //nolint:bodyclose // resp is nil on error; Post closes it before returning
	require.Error(t, err)
	assert.Nil(t, resp)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
}

func TestPost_Unauthorized(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	resp, err := Post(server.URL, "", map[string]string{}) //nolint:bodyclose // resp is nil on error; Post closes it before returning
	require.Error(t, err)
	assert.Nil(t, resp)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.StatusCode)
}

func TestPost_NetworkError(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	_, err := Post("http://127.0.0.1:1/does-not-exist", "", map[string]string{}) //nolint:bodyclose // network error path returns nil resp
	require.Error(t, err)
}

func TestPost_SetsHeaders(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	var (
		gotAuth        string
		gotContentType string
		gotUserAgent   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotUserAgent = r.Header.Get("User-Agent")

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	resp, err := Post(server.URL, "", map[string]string{})
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, "Bearer test-token", gotAuth)
	assert.Equal(t, "application/json", gotContentType)
	assert.NotEmpty(t, gotUserAgent)
}

func TestPost_SendsBody(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		gotBody, err = io.ReadAll(r.Body)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	body := map[string]any{"name": "my-agent", "count": 3}

	resp, err := Post(server.URL, "", body)
	require.NoError(t, err)

	defer resp.Body.Close()

	expected, err := json.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(gotBody))
}

func TestPostJSON_Decode(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc","name":"my-agent"}`))
	}))
	defer server.Close()

	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	err := PostJSON(server.URL, "", map[string]string{}, &out)
	require.NoError(t, err)
	assert.Equal(t, "abc", out.ID)
	assert.Equal(t, "my-agent", out.Name)
}

func TestPostJSON_DecodeError(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	var out struct{}

	err := PostJSON(server.URL, "", map[string]string{}, &out)
	require.Error(t, err)
}
