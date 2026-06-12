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

// TestPatchJSON_204NoContent guards the same shape of bug as
// TestDeleteJSON_204NoContent: isPatchSuccess accepts 204 but the JSON
// decode would surface io.EOF on an empty body, masking a successful
// patch from any caller that passes non-nil v.
func TestPatchJSON_204NoContent(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out struct {
		ID string `json:"id"`
	}

	err := PatchJSON(server.URL, "", map[string]string{"k": "v"}, &out)
	require.NoError(t, err)
	assert.Empty(t, out.ID)
}

func TestPatchJSON_200WithBody(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"xyz"}`))
	}))
	defer server.Close()

	var out struct {
		ID string `json:"id"`
	}

	err := PatchJSON(server.URL, "", map[string]string{"k": "v"}, &out)
	require.NoError(t, err)
	assert.Equal(t, "xyz", out.ID)
}

// TestPatchJSON_ErrorCarriesBodyDetail pins Patch to ErrFromResp on failure
// so the server's error detail reaches the user instead of a bare status code.
func TestPatchJSON_ErrorCarriesBodyDetail(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"resource is immutable"}`))
	}))
	defer server.Close()

	err := PatchJSON(server.URL, "", map[string]string{"k": "v"}, nil)
	require.Error(t, err)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "resource is immutable")
}
