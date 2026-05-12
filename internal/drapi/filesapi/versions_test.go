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

package filesapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListVersions_SinglePage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "-created", r.URL.Query().Get("orderBy"))

		page := CatalogVersionsResp{
			Data: []CatalogVersion{
				{ID: "v3", CreatedAt: "2026-04-10T14:30:00Z", NumFiles: 47, TotalSize: 2412544},
				{ID: "v2", CreatedAt: "2026-04-10T10:15:00Z", NumFiles: 46, TotalSize: 2300000},
				{ID: "v1", CreatedAt: "2026-04-09T16:45:00Z", NumFiles: 45, TotalSize: 2100000},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(page))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 0)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, "v3", got[0].ID)
	assert.Equal(t, 47, got[0].NumFiles)
	assert.Equal(t, int64(2412544), got[0].TotalSize)
}

func TestListVersions_Pagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")

		w.Header().Set("Content-Type", "application/json")

		if offset == "" {
			page1 := CatalogVersionsResp{
				Data: []CatalogVersion{
					{ID: "v5", CreatedAt: "2026-04-10T14:30:00Z", NumFiles: 5, TotalSize: 500},
					{ID: "v4", CreatedAt: "2026-04-10T13:30:00Z", NumFiles: 4, TotalSize: 400},
				},
			}

			page1.Next = "http://" + r.Host + r.URL.Path + "?offset=2"
			assert.NoError(t, json.NewEncoder(w).Encode(page1))

			return
		}

		page2 := CatalogVersionsResp{
			Data: []CatalogVersion{
				{ID: "v3", CreatedAt: "2026-04-10T12:30:00Z", NumFiles: 3, TotalSize: 300},
			},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(page2))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 0)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "v5", got[0].ID)
	assert.Equal(t, "v3", got[2].ID)
}

func TestListVersions_LimitTruncatesMidPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, _ *http.Request) {
		page := CatalogVersionsResp{
			Data: []CatalogVersion{
				{ID: "v5"}, {ID: "v4"}, {ID: "v3"}, {ID: "v2"}, {ID: "v1"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(page))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v5", got[0].ID)
	assert.Equal(t, "v4", got[1].ID)
}

func TestListVersions_RejectsCrossHostNext(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, _ *http.Request) {
		page := CatalogVersionsResp{
			Data: []CatalogVersion{{ID: "v1"}},
		}
		page.Next = "https://attacker.example/api/v2/files/cid-1/versions/?offset=1"

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(page))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host")
	assert.Nil(t, got)
}

// TestListVersions_DecodesRealServerShape pins the JSON field names used
// by the FilesAPI versions endpoint (catalogVersionId, creationDate,
// numFiles, size) so a future field rename in CatalogVersion can't
// silently break decoding.
func TestListVersions_DecodesRealServerShape(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"count": 1,
			"totalCount": 1,
			"next": null,
			"previous": null,
			"data": [{
				"catalogId": "cid-1",
				"catalogVersionId": "ver-abc-1",
				"numFiles": 3,
				"creationDate": "2026-04-30T13:54:38.562000Z",
				"isLatest": true,
				"createdBy": "test",
				"isStage": false,
				"size": 615
			}]
		}`))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ver-abc-1", got[0].ID)
	assert.Equal(t, "2026-04-30T13:54:38.562000Z", got[0].CreatedAt)
	assert.Equal(t, 3, got[0].NumFiles)
	assert.Equal(t, int64(615), got[0].TotalSize)
}

func TestListVersions_EmptyResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(CatalogVersionsResp{}))
	})

	startServer(t, mux)

	got, err := New().ListVersions("cid-1", 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}
