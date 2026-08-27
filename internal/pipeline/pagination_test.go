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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListPipelines_WalksNextLinks covers the multi-page walk: the first
// request carries the clamped page size, following next-links fetches the
// rest, and the returned envelope keeps the server's true TotalCount so
// renderers can still show "Showing N of <total>".
func TestListPipelines_WalksNextLinks(t *testing.T) {
	installSkipAuth(t)

	var serverURL string

	requests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("offset") == "" {
			next := serverURL + "/api/v2/pipelines?offset=2"
			_, _ = w.Write([]byte(`{"data":[{"id":"p-1","name":"one","mode":"draft","isActive":true},` +
				`{"id":"p-2","name":"two","mode":"draft","isActive":true}],"totalCount":3,"count":2,"next":"` + next + `"}`))

			return
		}

		_, _ = w.Write([]byte(`{"data":[{"id":"p-3","name":"three","mode":"draft","isActive":true}],"totalCount":3,"count":1,"next":null}`))
	}))

	defer srv.Close()

	serverURL = srv.URL
	installEndpoint(t, srv.URL)

	page, err := ListPipelines("", "", 0, 500)
	require.NoError(t, err)

	require.Len(t, page.Data, 3)
	assert.Equal(t, "p-3", page.Data[2].PipelineID)
	assert.Equal(t, 3, page.TotalCount, "TotalCount stays the server's total, not the returned row count")
	assert.Equal(t, 2, requests)
}

func TestListRuns_WalksNextLinks(t *testing.T) {
	installSkipAuth(t)

	var serverURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("offset") == "" {
			next := serverURL + "/api/v2/pipelines/p-1/dispatches?offset=1"
			_, _ = w.Write([]byte(`{"data":[{"id":"d-1","pipelineId":"p-1","inputId":"in-1","triggeredBy":"u","status":"RUNNING"}],"totalCount":2,"count":1,"next":"` + next + `"}`))

			return
		}

		_, _ = w.Write([]byte(`{"data":[{"id":"d-2","pipelineId":"p-1","inputId":"in-1","triggeredBy":"u","status":"COMPLETED"}],"totalCount":2,"count":1,"next":null}`))
	}))

	defer srv.Close()

	serverURL = srv.URL
	installEndpoint(t, srv.URL)

	items, err := ListRuns("p-1", ScopeDraft, nil, 0, 5)
	require.NoError(t, err)

	require.Len(t, items, 2)
	assert.Equal(t, RunStatusRunning, items[0].Status)
	assert.Equal(t, RunStatusCompleted, items[1].Status)
}

func TestListPipelines_RejectsInvalidPagination(t *testing.T) {
	for _, tc := range []struct {
		offset int
		limit  int
	}{
		{offset: 0, limit: 0},
		{offset: 0, limit: -1},
		{offset: -1, limit: 5},
	} {
		_, err := ListPipelines("", "", tc.offset, tc.limit)
		require.Errorf(t, err, "offset %d limit %d", tc.offset, tc.limit)
		assert.Contains(t, err.Error(), "must be")
	}
}
