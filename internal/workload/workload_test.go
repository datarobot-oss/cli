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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkloadStatuses(t *testing.T) {
	t.Run("valid values are lowercased and trimmed", func(t *testing.T) {
		parsed, err := ParseWorkloadStatuses([]string{"Running", " stopped ", "ERRORED"})
		require.NoError(t, err)
		assert.Equal(t, []string{"running", "stopped", "errored"}, parsed)
	})

	t.Run("empty entries are skipped", func(t *testing.T) {
		parsed, err := ParseWorkloadStatuses([]string{"", "running"})
		require.NoError(t, err)
		assert.Equal(t, []string{"running"}, parsed)
	})

	t.Run("unknown value errors", func(t *testing.T) {
		_, err := ParseWorkloadStatuses([]string{"sleeping"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `invalid status "sleeping"`)
		assert.Contains(t, err.Error(), WorkloadStatusRunning)
	})
}

func TestValidateWorkloadCreateRequest(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name: "valid with artifactId",
			spec: `{"name": "wl", "artifactId": "abc123", "runtime": {}}`,
		},
		{
			name: "valid with inline artifact",
			spec: `{"name": "wl", "artifact": {"name": "art", "type": "service", "spec": {}}}`,
		},
		{
			name:    "missing name",
			spec:    `{"artifactId": "abc123"}`,
			wantErr: "required field 'name'",
		},
		{
			name:    "neither artifactId nor artifact",
			spec:    `{"name": "wl"}`,
			wantErr: "exactly one of 'artifactId'",
		},
		{
			name:    "both artifactId and artifact",
			spec:    `{"name": "wl", "artifactId": "abc", "artifact": {"name": "art"}}`,
			wantErr: "exactly one of 'artifactId'",
		},
		{
			name: "null artifact counts as absent",
			spec: `{"name": "wl", "artifactId": "abc", "artifact": null}`,
		},
		{
			name:    "invalid JSON",
			spec:    `{"name": `,
			wantErr: "invalid spec",
		},
		{
			name: "unknown fields pass through to the server",
			spec: `{"name": "wl", "artifactId": "abc", "futureField": true}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateWorkloadCreateRequest([]byte(c.spec))

			if c.wantErr == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

// serverWorkloadDoc is a realistic workload document including server-side
// extras the projection must ignore (owners, permissions, runtime).
func serverWorkloadDoc(id, name, status string) string {
	return fmt.Sprintf(`{
		"id": %q,
		"name": %q,
		"status": %q,
		"type": "service",
		"importance": "low",
		"artifactId": "art-1",
		"endpoint": "https://app.example.com/api/v2/endpoints/workloads/%s/",
		"createdAt": "2026-06-10T08:00:00Z",
		"updatedAt": "2026-06-10T08:05:00Z",
		"owners": [{"id": "u-1", "email": "pii@example.com"}],
		"permissions": ["CAN_DELETE"],
		"runtime": {"containerGroups": []}
	}`, id, name, status, id)
}

func assertProjection(t *testing.T, w *Workload, id, name, status string) {
	t.Helper()

	assert.Equal(t, id, w.ID)
	assert.Equal(t, name, w.Name)
	assert.Equal(t, status, w.Status)
	assert.Equal(t, "service", w.Type)
	assert.Equal(t, "low", w.Importance)
	assert.Equal(t, "art-1", w.ArtifactID)
	assert.Equal(t, "https://app.example.com/api/v2/endpoints/workloads/"+id+"/", w.Endpoint)
	assert.Equal(t, time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC), w.CreatedAt.UTC())
}

func TestCreateWorkload_PostsSpecVerbatimAndParses201(t *testing.T) {
	installSkipAuth(t)

	spec := []byte(`{"name": "wl-1", "artifactId": "art-1", "futureField": {"passes": true}}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/workloads/", r.URL.Path)

		// assert (not require) inside the handler: require calls t.FailNow,
		// which is illegal off the test goroutine (testifylint go-require).
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		// The spec file bytes are sent verbatim, unknown fields included.
		assert.JSONEq(t, string(spec), string(body))

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, serverWorkloadDoc("wl-id-1", "wl-1", "submitted"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	workload, err := CreateWorkload(json.RawMessage(spec))
	require.NoError(t, err)
	assertProjection(t, workload, "wl-id-1", "wl-1", "submitted")
}

func TestCreateWorkload_422SurfacesServerDetail(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail":[{"path":"artifactId","message":"Artifact not found","code":"invalid"}]}`)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := CreateWorkload(json.RawMessage(`{"name":"wl","artifactId":"missing"}`))
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "Artifact not found")
}

func TestGetWorkload_Success(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-id-1/", r.URL.Path)
		fmt.Fprint(w, serverWorkloadDoc("wl-id-1", "wl-1", "running"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	workload, err := GetWorkload("wl-id-1")
	require.NoError(t, err)
	assertProjection(t, workload, "wl-id-1", "wl-1", "running")
}

func TestGetWorkload_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// '?' must arrive escaped inside the path segment, never as a query.
		assert.Equal(t, "/api/v2/workloads/wl-1%3Fforce=true/", r.URL.EscapedPath())
		assert.Empty(t, r.URL.RawQuery)
		fmt.Fprint(w, serverWorkloadDoc("wl-id-1", "wl-1", "running"))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkload("wl-1?force=true")
	require.NoError(t, err)
}

func TestGetWorkload_404PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetWorkload("missing")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func workloadListPage(next string, docs ...string) string {
	nextJSON := "null"
	if next != "" {
		nextJSON = fmt.Sprintf("%q", next)
	}

	return fmt.Sprintf(
		`{"data": [%s], "count": %d, "totalCount": %d, "next": %s, "previous": null}`,
		joinDocs(docs), len(docs), len(docs), nextJSON,
	)
}

func joinDocs(docs []string) string {
	return strings.Join(docs, ",")
}

func TestListWorkloads_SinglePageWithStatusFilter(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/", r.URL.Path)
		assert.Equal(t, "25", r.URL.Query().Get("limit"))
		assert.Equal(t, []string{"running", "errored"}, r.URL.Query()["status"])
		fmt.Fprint(w, workloadListPage("", serverWorkloadDoc("wl-1", "a", "running")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	workloads, err := ListWorkloads(25, []string{"running", "errored"})
	require.NoError(t, err)
	require.Len(t, workloads, 1)
	assert.Equal(t, "wl-1", workloads[0].ID)
}

func TestListWorkloads_ClampsPageSizeToServerMax(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server rejects limit > 100 with a 422, so the page size must
		// be clamped even when --limit asks for more.
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		fmt.Fprint(w, workloadListPage("", serverWorkloadDoc("wl-1", "a", "running")))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := ListWorkloads(250, nil)
	require.NoError(t, err)
}

func TestListWorkloads_FollowsNextAndTruncatesToLimit(t *testing.T) {
	installSkipAuth(t)

	var srvURL string

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		switch calls {
		case 1:
			next := srvURL + "/api/v2/workloads/?offset=2&limit=3"
			fmt.Fprint(w, workloadListPage(next,
				serverWorkloadDoc("wl-1", "a", "running"),
				serverWorkloadDoc("wl-2", "b", "running"),
			))
		default:
			fmt.Fprint(w, workloadListPage("",
				serverWorkloadDoc("wl-3", "c", "running"),
				serverWorkloadDoc("wl-4", "d", "running"),
			))
		}
	}))

	defer srv.Close()

	srvURL = srv.URL

	installEndpoint(t, srv.URL)

	workloads, err := ListWorkloads(3, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	require.Len(t, workloads, 3)
	assert.Equal(t, "wl-3", workloads[2].ID)
}

func TestListWorkloads_RejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		_, err := ListWorkloads(limit, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	}
}

func TestDeleteWorkload_Success(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/workloads/wl-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteWorkload("wl-1"))
}

func TestDeleteWorkload_EscapesIDInPath(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// '/' must arrive as %2F so a traversal-looking id stays one segment.
		assert.Equal(t, "/api/v2/workloads/..%2Fother/", r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteWorkload("../other"))
}

func TestDeleteWorkload_404PropagatesAsHTTPError(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := DeleteWorkload("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestNewWorkloadOutput_FormatsTimestampsRFC3339(t *testing.T) {
	w := Workload{
		ID:         "wl-1",
		Name:       "a",
		Status:     "running",
		Type:       "service",
		Importance: "low",
		ArtifactID: "art-1",
		Endpoint:   "https://e/",
		CreatedAt:  time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 6, 10, 8, 5, 0, 0, time.UTC),
	}

	out := NewWorkloadOutput(w)
	assert.Equal(t, "2026-06-10T08:00:00Z", out.CreatedAt)
	assert.Equal(t, "2026-06-10T08:05:00Z", out.UpdatedAt)
	assert.Equal(t, "https://e/", out.Endpoint)
}
