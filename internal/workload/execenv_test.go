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
// eeDocLang is eeDoc with a programmingLanguage, for the sorting tests.
func eeDocLang(id, name, language, versionID string) string {
	return fmt.Sprintf(
		`{"id": %q, "name": %q, "programmingLanguage": %q, "latestSuccessfulVersion": {"id": %q}}`,
		id, name, language, versionID,
	)
}

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

// Environment names are not unique: the platform catalog and a tenant's own
// copies can share one. Picking the server's first hit would build against the
// wrong base image and surface as a puzzling runtime failure, so an ambiguous
// name is an error naming the candidates.
func TestResolveExecutionEnvironment_AmbiguousName(t *testing.T) {
	installSkipAuth(t)

	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprintf(w, `{"data": [%s], "next": ""}`, eeDoc("ee-2", "Python 3.11", "ver-2"))

			return
		}

		fmt.Fprintf(w, `{"data": [%s], "next": %q}`,
			eeDoc("ee-1", "Python 3.11", "ver-1"),
			srv.URL+"/api/v2/executionEnvironments/?page=2",
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ResolveExecutionEnvironment("Python 3.11")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "ee-1")
	assert.Contains(t, err.Error(), "ee-2", "the duplicate on a later page counts too")
}

// Ids are unique, so an id match is answered from the page it appears on
// without scanning the rest for name collisions.
func TestResolveExecutionEnvironment_IDWinsOverDuplicateNames(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s, %s], "next": ""}`,
			eeDoc("ee-1", "Shared", "ver-1"),
			eeDoc("ee-2", "Shared", "ver-2"),
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	id, versionID, err := ResolveExecutionEnvironment("ee-2")
	require.NoError(t, err)
	assert.Equal(t, "ee-2", id)
	assert.Equal(t, "ver-2", versionID)
}

// Unlike the limit-bounded listings this one scans for a match, so a server
// that keeps handing back a next cursor has no natural stopping point. The cap
// turns a hung command into an error.
func TestResolveExecutionEnvironment_StopsAtPageCap(t *testing.T) {
	installSkipAuth(t)

	var (
		srv   *httptest.Server
		pages int
	)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++

		fmt.Fprintf(w, `{"data": [%s], "next": %q}`,
			eeDoc("ee-1", "Other", "ver-1"),
			srv.URL+"/api/v2/executionEnvironments/?limit=100",
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, _, err := ResolveExecutionEnvironment("Wanted")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pages")
	assert.Equal(t, maxExecEnvPages, pages, "the walk stops at the cap rather than running forever")
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

// The base-image picker offers only what a build can actually target: an
// environment with no successful version would fail at build time with
// nothing the user could do about it.
func TestListExecutionEnvironments_SkipsVersionlessEnvironments(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s, %s, %s], "next": ""}`,
			eeDoc("ee-1", "Python 3.11", "ver-1"),
			eeDoc("ee-2", "Never built", ""),
			eeDoc("ee-3", "Python 3.12", "ver-3"),
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	environments, err := ListExecutionEnvironments(50)
	require.NoError(t, err)

	require.Len(t, environments, 2)
	assert.Equal(t, "ee-1", environments[0].ID)
	assert.Equal(t, "ee-3", environments[1].ID)
}

func TestListExecutionEnvironments_DrainsSortsAndTrimsDeterministically(t *testing.T) {
	installSkipAuth(t)

	// next is filled in once the server has an address; the handler only ever
	// echoes this test's own value, never anything from the request.
	var next string

	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++

		switch pages {
		case 1:
			fmt.Fprintf(w, `{"data": [%s, %s], "next": %q}`,
				eeDocLang("ee-pyz", "Zephyr", "python", "v1"),
				eeDocLang("ee-oth", "Aardvark", "other", "v2"),
				next,
			)
		default:
			fmt.Fprintf(w, `{"data": [%s, %s], "next": ""}`,
				eeDocLang("ee-pya", "apex", "Python", "v3"),
				eeDocLang("ee-r", "Stats", "r", "v4"),
			)
		}
	}))

	defer srv.Close()

	next = srv.URL + "/api/v2/executionEnvironments/?limit=100&page=2"

	installEndpoint(t, srv.URL)

	environments, err := ListExecutionEnvironments(3)
	require.NoError(t, err)

	// Every page is read BEFORE trimming: an unsorted early stop would keep
	// whichever environments the server listed first, an arbitrary subset.
	assert.Equal(t, 2, pages, "the whole listing is drained before sorting and trimming")

	// Languages cluster (case-insensitively), names sort within a language,
	// and the trim cuts the SORTED list — here dropping the catch-all
	// "other" entry, the least informative group, not a random page-two row.
	require.Len(t, environments, 3)
	assert.Equal(t, []string{"ee-pya", "ee-pyz", "ee-r"},
		[]string{environments[0].ID, environments[1].ID, environments[2].ID})
}

func TestListExecutionEnvironments_ClustersLanguagesWithOtherLast(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"data": [%s, %s, %s, %s, %s], "next": ""}`,
			eeDocLang("ee-other", "AAA first by name alone", "other", "v1"),
			eeDocLang("ee-none", "BBB unlabeled", "", "v2"),
			eeDocLang("ee-r", "R Lab", "r", "v3"),
			eeDocLang("ee-java", "JVM", "java", "v4"),
			eeDocLang("ee-py", "Py", "python", "v5"),
		)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	environments, err := ListExecutionEnvironments(50)
	require.NoError(t, err)
	require.Len(t, environments, 5)

	got := make([]string, 0, len(environments))
	for _, ee := range environments {
		got = append(got, ee.ID)
	}

	// java < python < r, then the catch-all and the unlabeled at the end by
	// name — "other" is where the platform files what it cannot name, so it
	// belongs after the languages that say something.
	assert.Equal(t, []string{"ee-java", "ee-py", "ee-r", "ee-other", "ee-none"}, got)
}

func TestListExecutionEnvironments_ServerError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := ListExecutionEnvironments(50)
	require.Error(t, err)
}
