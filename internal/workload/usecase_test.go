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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadUseCases_ReadsOneFilteredPage(t *testing.T) {
	var calls atomic.Int32

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/useCases/", r.URL.Path)
		assert.Equal(t, "wl-1", r.URL.Query().Get("entityId"))
		assert.Equal(t, "workload", r.URL.Query().Get("entityType"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		assert.Equal(t, "0", r.URL.Query().Get("offset"))

		fmt.Fprint(w, `{"data":[{"id":"uc-1"},{"id":"uc-2"}],"next":null,"totalCount":2}`)
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Equal(t, []string{"uc-1", "uc-2"}, links.IDs)
	assert.True(t, links.Has("uc-2"))
	assert.False(t, links.Has("uc-3"))
	assert.Equal(t, int32(1), calls.Load(), "a short page is the last")
}

// The next link drops the entity filter, so every page is asked for again
// with the filter and the next offset, never by following next.
func TestWorkloadUseCases_PagesWithTheFilterKept(t *testing.T) {
	var offsets []string

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "wl-1", r.URL.Query().Get("entityId"))
		assert.Equal(t, "workload", r.URL.Query().Get("entityType"))

		offset := r.URL.Query().Get("offset")
		offsets = append(offsets, offset)

		next := `"http://elsewhere/api/v2/useCases/?offset=100&limit=100"`

		switch offset {
		case "0":
			fmt.Fprintf(w, `{"data":[%s],"next":%s,"totalCount":101}`, ucIDs(0, 100), next)
		case "100":
			fmt.Fprint(w, `{"data":[{"id":"uc-100"}],"next":null,"totalCount":101}`)
		default:
			t.Errorf("unexpected offset %q", offset)
		}
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Equal(t, []string{"0", "100"}, offsets)
	assert.Len(t, links.IDs, 101)
	assert.True(t, links.Has("uc-100"), "a link past the first page is seen")
}

// A full last page is the end when totalCount says so, with no extra request.
func TestWorkloadUseCases_StopsAtTotalCount(t *testing.T) {
	var calls atomic.Int32

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"data":[%s],"next":null,"totalCount":100}`, ucIDs(0, 100))
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Len(t, links.IDs, 100)
	assert.Equal(t, int32(1), calls.Load())
}

func TestWorkloadUseCases_NoLinks(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[],"next":null,"totalCount":0}`)
	}))

	links, err := WorkloadUseCases("wl-1")
	require.NoError(t, err)

	assert.Empty(t, links.IDs)
}

// A server that never stops answering full pages does not hold a deploy.
func TestWorkloadUseCases_BoundsThePages(t *testing.T) {
	var calls atomic.Int32

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"data":[%s]}`, ucIDs(0, 100))
	}))

	_, err := WorkloadUseCases("wl-1")
	require.Error(t, err)
	assert.Equal(t, int32(useCaseLinkPages), calls.Load())
}

func TestWorkloadUseCases_ReadErrorFails(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"forbidden"}`)
	}))

	_, err := WorkloadUseCases("wl-1")
	require.Error(t, err)
}

// ucIDs is n link items, uc-<from> onwards, as JSON array members.
func ucIDs(from, n int) string {
	items := make([]string, 0, n)

	for i := from; i < from+n; i++ {
		items = append(items, fmt.Sprintf(`{"id":"uc-%d"}`, i))
	}

	return strings.Join(items, ",")
}

func TestLinkWorkloadToUseCase_PostsTheLinkRoute(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/useCases/uc-1/workloads/wl-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, LinkWorkloadToUseCase("uc-1", "wl-1"))
}

// A refusal is a refusal whatever it says: the read before the link sees
// every link, so the run never posts one that already exists.
func TestLinkWorkloadToUseCase_AlreadyLinkedIsNotSwallowed(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Entity is already linked to this Use Case"}`)
	}))

	require.Error(t, LinkWorkloadToUseCase("uc-1", "wl-1"))
}

func TestLinkWorkloadToUseCase_RefusalKeepsTheStatus(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail":{"code":"ENCLAVE_TARGETING_REQUIRED","message":"Set a selection policy"}}`)
	}))

	err := LinkWorkloadToUseCase("uc-1", "wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "ENCLAVE_TARGETING_REQUIRED")
}

func TestUseCaseEnclaveCount_ReadsTheUseCase(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/useCases/uc-1/", r.URL.Path)
		fmt.Fprint(w, `{"id":"uc-1","enclavesCount":2}`)
	}))

	count, err := UseCaseEnclaveCount("uc-1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestUseCaseEnclaveCount_MissingOrNullIsNone(t *testing.T) {
	for _, body := range []string{`{"id":"uc-1"}`, `{"id":"uc-1","enclavesCount":null}`} {
		serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, body)
		}))

		count, err := UseCaseEnclaveCount("uc-1")
		require.NoError(t, err)
		assert.Zero(t, count, body)
	}
}

func TestUseCaseLinkRequest_NeedsBothIDs(t *testing.T) {
	require.Error(t, LinkWorkloadToUseCase("", "wl-1"))
	require.Error(t, LinkWorkloadToUseCase("uc-1", " "))
}
