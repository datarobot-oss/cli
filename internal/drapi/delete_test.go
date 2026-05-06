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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeleteJSON_204NoContent guards the regression where a 204 response
// (accepted by isDeleteSuccess) was decoded anyway and surfaced as
// io.EOF — making a successful delete look like a failure to callers
// that pass a non-nil v (e.g. DeleteFiles always passes &resp).
func TestDeleteJSON_204NoContent(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out struct {
		ID string `json:"id"`
	}

	err := DeleteJSON(server.URL, "", nil, &out)
	require.NoError(t, err)
	assert.Empty(t, out.ID)
}

func TestDeleteJSON_200WithBody(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer server.Close()

	var out struct {
		ID string `json:"id"`
	}

	err := DeleteJSON(server.URL, "", nil, &out)
	require.NoError(t, err)
	assert.Equal(t, "abc", out.ID)
}

func TestDeleteJSON_NonSuccess(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var out struct{}

	err := DeleteJSON(server.URL, "", nil, &out)
	require.Error(t, err)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}
