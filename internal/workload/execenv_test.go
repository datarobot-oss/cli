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

package workload

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eeDoc is one execution environment as the server serializes it.
func eeDoc(id, name, versionID string) string {
	if versionID == "" {
		return fmt.Sprintf(`{"id": %q, "name": %q, "latestSuccessfulVersion": null}`, id, name)
	}

	return fmt.Sprintf(
		`{"id": %q, "name": %q, "latestSuccessfulVersion": {"id": %q}}`,
		id, name, versionID,
	)
}

func TestResolveExecutionEnvironment_MatchesByName(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s, %s], "next": ""}`,
			eeDoc("ee-1", "Python 3.11", "ver-1"),
			eeDoc("ee-2", "Python 3.12", "ver-2"),
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	id, versionID, err := ResolveExecutionEnvironment("Python 3.12")
	require.NoError(t, err)
	assert.Equal(t, "ee-2", id)
	assert.Equal(t, "ver-2", versionID)
}

func TestResolveExecutionEnvironment_MatchesByID(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s], "next": ""}`, eeDoc("ee-1", "Python 3.11", "ver-1"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	id, versionID, err := ResolveExecutionEnvironment("ee-1")
	require.NoError(t, err)
	assert.Equal(t, "ee-1", id)
	assert.Equal(t, "ver-1", versionID)
}

// The match may live past the first page, so a miss on page one must follow
// next rather than reporting not-found.
func TestResolveExecutionEnvironment_FollowsNextPage(t *testing.T) {
	installSkipAuth(t)

	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprintf(w, `{"data": [%s], "next": ""}`, eeDoc("ee-9", "Wanted", "ver-9"))

			return
		}

		fmt.Fprintf(w, `{"data": [%s], "next": %q}`,
			eeDoc("ee-1", "Other", "ver-1"),
			srv.URL+"/api/v2/executionEnvironments/?page=2",
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	id, versionID, err := ResolveExecutionEnvironment("Wanted")
	require.NoError(t, err)
	assert.Equal(t, "ee-9", id)
	assert.Equal(t, "ver-9", versionID)
}

// An environment whose builds have all failed cannot be built from, and the
// error must say that rather than "not found", which would send the user
// looking for a typo.
func TestResolveExecutionEnvironment_NoSuccessfulVersion(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s], "next": ""}`, eeDoc("ee-1", "Broken", ""))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ResolveExecutionEnvironment("Broken")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no successful version")
}

func TestResolveExecutionEnvironment_NotFound(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s], "next": ""}`, eeDoc("ee-1", "Python 3.11", "ver-1"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ResolveExecutionEnvironment("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nope" not found`)
}

// A next link pointing at another host is an SSRF vector, so pagination must
// refuse to follow it.
func TestResolveExecutionEnvironment_RejectsCrossHostNext(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s], "next": "https://evil.example.com/api/v2/executionEnvironments/"}`,
			eeDoc("ee-1", "Other", "ver-1"),
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ResolveExecutionEnvironment("Wanted")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not found", "a cross-host next must fail loudly, not fall through to not-found")
}
