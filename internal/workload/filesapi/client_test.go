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
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startServer wires the httptest server's URL into viper so all
// config.GetEndpointURL calls land on the test handler. It also serves
// the /version/ endpoint that config.GetAPIKey hits — without that, every
// upload-style call would fail before reaching the test handler.
func startServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/version/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	})

	mux.Handle("/api/v2/", http.StripPrefix("", handler))

	srv := httptest.NewServer(mux)

	previousURL := viperx.GetString(config.DataRobotURL)
	previousKey := viperx.GetString(config.DataRobotAPIKey)

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		srv.Close()
		viperx.Set(config.DataRobotURL, previousURL)
		viperx.Set(config.DataRobotAPIKey, previousKey)
	})

	return srv
}

func TestCreateCatalog(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/files/", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v0"}`))
	}))

	c := NewClient()
	got, err := c.CreateCatalog()

	require.NoError(t, err)
	assert.Equal(t, "cid-1", got.CatalogID)
	assert.Equal(t, "v0", got.CatalogVersionID)
}

func TestCreateStage_ApplyStage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/stages/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","stageId":"st-1"}`))
	})
	mux.HandleFunc("/api/v2/files/cid-1/fromStage/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		var req ApplyStageReq

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "st-1", req.StageID)
		assert.Equal(t, OverwriteReplace, req.Overwrite)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v1","numFiles":3}`))
	})

	startServer(t, mux)

	c := NewClient()

	stage, err := c.CreateStage("cid-1")
	require.NoError(t, err)
	assert.Equal(t, "st-1", stage.StageID)

	apply, err := c.ApplyStage("cid-1", "st-1", OverwriteReplace)
	require.NoError(t, err)
	assert.Equal(t, "v1", apply.CatalogVersionID)
	assert.Equal(t, 3, apply.NumFiles)
}

func TestUploadToStage_Multipart(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/files/cid-1/stages/st-1/upload/", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		mr, err := r.MultipartReader()
		if !assert.NoError(t, err) {
			return
		}

		part, err := mr.NextPart()
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, "file", part.FormName())
		assert.Equal(t, "agent.py", part.FileName())

		body, err := io.ReadAll(part)
		assert.NoError(t, err)
		assert.Equal(t, "print('hi')\n", string(body))

		// Expect no further parts.
		_, err = mr.NextPart()
		assert.ErrorIs(t, err, io.EOF)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","stageId":"st-1"}`))
	}))

	c := NewClient()
	body := strings.NewReader("print('hi')\n")
	err := c.UploadToStage("cid-1", "st-1", "agent.py", int64(body.Len()), body)
	require.NoError(t, err)
}

// TestUploadToStage_AdvertisesContentLength verifies that the size
// passed to UploadToStage is forwarded as an exact Content-Length on
// the wire — proving the body is no longer buffered in-memory and that
// the size parameter is actually consumed (not silently dropped).
func TestUploadToStage_AdvertisesContentLength(t *testing.T) {
	const payload = "hello-world"

	var (
		gotContentLength int64
		gotBodyLen       int
	)

	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength

		raw, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		gotBodyLen = len(raw)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","stageId":"st-1"}`))
	}))

	c := NewClient()
	body := strings.NewReader(payload)
	require.NoError(t, c.UploadToStage("cid-1", "st-1", "test.txt", int64(body.Len()), body))

	assert.Greater(t, gotContentLength, int64(len(payload)),
		"Content-Length should include multipart envelope, not just payload")
	assert.Equal(t, int64(gotBodyLen), gotContentLength,
		"advertised Content-Length must match received body length")
}

func TestAllFiles_Pagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/v1/allFiles/", func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")

		w.Header().Set("Content-Type", "application/json")

		if offset == "" {
			page1 := AllFilesResp{
				Data: []AllFilesItem{
					{FileName: "a.py", FileSize: 10, FileChecksum: "aaa"},
					{FileName: "b.py", FileSize: 20, FileChecksum: "bbb"},
				},
			}

			// The client copies Next verbatim into pageURL; we make Next a
			// fully-qualified URL so the next request lands here too.
			page1.Next = "http://" + r.Host + r.URL.Path + "?offset=1"
			assert.NoError(t, json.NewEncoder(w).Encode(page1))

			return
		}

		page2 := AllFilesResp{
			Data: []AllFilesItem{
				{FileName: "café.py", FileSize: 30, FileChecksum: "ccc"},
			},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(page2))
	})

	startServer(t, mux)

	c := NewClient()

	got, err := c.AllFiles("cid-1", "v1")
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.Equal(t, FileMeta{Hash: "aaa", Size: 10}, got["a.py"])
	assert.Equal(t, FileMeta{Hash: "ccc", Size: 30}, got["café.py"])
}

// TestAllFiles_RejectsCrossHostNext confirms that a pagination cursor
// pointing at a different host is rejected before drapi.GetJSON can
// attach the bearer token to it. Without this gate, a compromised or
// buggy server could exfiltrate the token by setting Next to an
// attacker-controlled origin.
func TestAllFiles_RejectsCrossHostNext(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/v1/allFiles/", func(w http.ResponseWriter, _ *http.Request) {
		page := AllFilesResp{
			Data: []AllFilesItem{
				{FileName: "ok.py", FileSize: 1, FileChecksum: "aa"},
			},
		}
		page.Next = "https://attacker.example/api/v2/files/cid-1/versions/v1/allFiles/?offset=1"
		assert.NoError(t, json.NewEncoder(w).Encode(page))
	})

	startServer(t, mux)

	c := NewClient()

	got, err := c.AllFiles("cid-1", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host")
	assert.Nil(t, got)
}

// TestAllFiles_RejectsHostilePath confirms a server-supplied path that
// escapes the project root (".." segments or absolute) is rejected at the
// boundary, so engine code can't subsequently filepath.Join it onto the
// project directory and write/delete outside the tree.
func TestAllFiles_RejectsHostilePath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/files/cid-1/versions/v1/allFiles/", func(w http.ResponseWriter, _ *http.Request) {
		page := AllFilesResp{
			Data: []AllFilesItem{
				{FileName: "ok.py", FileSize: 1, FileChecksum: "aa"},
				{FileName: "../../etc/passwd", FileSize: 1, FileChecksum: "bb"},
			},
		}
		assert.NoError(t, json.NewEncoder(w).Encode(page))
	})

	startServer(t, mux)

	c := NewClient()

	got, err := c.AllFiles("cid-1", "v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes project root")
	assert.Nil(t, got)
}

func TestDownloadFile_RejectsHostilePath(t *testing.T) {
	// No server fixture is needed — validation runs before the request is
	// ever dispatched.
	startServer(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("DownloadFile should reject hostile paths before reaching the server")
	}))

	c := NewClient()

	cases := []struct {
		name    string
		path    string
		wantSub string
	}{
		{"DotDotEscape", "../escape", "escapes project root"},
		{"BackslashTraversal", `..\escape`, "backslash"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			_, _, err := c.DownloadFile("cid-1", "v1", tc.path, &buf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestDeleteFiles(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/files/cid-1/allFiles/", r.URL.Path)

		var req DeleteFilesReq

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, []string{"old.py"}, req.Paths)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v2","numFiles":1,"results":[{"path":"old.py","numFilesDeleted":1}]}`))
	}))

	c := NewClient()
	got, err := c.DeleteFiles("cid-1", []string{"old.py"})
	require.NoError(t, err)
	assert.Equal(t, "v2", got.CatalogVersionID)
	assert.Equal(t, 1, got.NumFiles)
}

func TestDeleteFiles_EmptyIsNoop(t *testing.T) {
	c := NewClient()
	got, err := c.DeleteFiles("cid-1", nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPollStatus_RunningJSON(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/status/sid-1/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"RUNNING_TO_WORKERS","statusId":"sid-1"}`))
	}))

	c := NewClient()
	resp, err := c.PollStatus("sid-1")
	require.NoError(t, err)
	assert.Equal(t, StatusRunningToWorkers, resp.Status)
	assert.False(t, IsTerminalStatus(resp.Status))
}

func TestPollStatus_CompletedRedirect(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/status/sid-2/", r.URL.Path)
		w.Header().Set("Location", "/api/v2/catalogItems/cat-x/")
		w.WriteHeader(http.StatusSeeOther)
	}))

	c := NewClient()
	resp, err := c.PollStatus("sid-2")
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, resp.Status)
	assert.True(t, IsTerminalStatus(resp.Status))
}

func TestUploadFromZipExisting(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/files/cid-1/fromFile/", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("useArchiveContents"))
		assert.Equal(t, "REPLACE", r.URL.Query().Get("overwrite"))
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		mr, err := r.MultipartReader()
		if !assert.NoError(t, err) {
			return
		}

		part, err := mr.NextPart()
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, "file", part.FormName())
		assert.Equal(t, "changes.zip", part.FileName())

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v9","statusId":"sid-9"}`))
	}))

	c := NewClient()

	zipBody := bytes.NewReader([]byte("PK\x03\x04fake-zip"))
	resp, err := c.UploadFromZipExisting("cid-1", "changes.zip", "", int64(zipBody.Len()), zipBody)
	require.NoError(t, err)
	assert.Equal(t, "v9", resp.CatalogVersionID)
	assert.Equal(t, "sid-9", resp.StatusID)
}

// TestUploadFromZipNew_HitsFromFileEndpoint locks in the (post-2026-04-30)
// fix that the new-catalog-from-zip path posts to /files/fromFile/ rather
// than /files/. The bare /files/ endpoint silently created an empty catalog
// without extracting the zip, so smoke-tested syncs reported success but
// the remote was empty.
func TestUploadFromZipNew_HitsFromFileEndpoint(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/files/fromFile/", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("useArchiveContents"))
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		mr, err := r.MultipartReader()
		if !assert.NoError(t, err) {
			return
		}

		part, err := mr.NextPart()
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, "file", part.FormName())
		assert.Equal(t, "wapi-sync.zip", part.FileName())

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"new-cid","catalogVersionId":"new-ver","statusId":"sid-new"}`))
	}))

	c := NewClient()

	zipBody := bytes.NewReader([]byte("PK\x03\x04fake-zip"))
	resp, err := c.UploadFromZipNew("wapi-sync.zip", int64(zipBody.Len()), zipBody)
	require.NoError(t, err)
	assert.Equal(t, "new-cid", resp.CatalogID)
	assert.Equal(t, "new-ver", resp.CatalogVersionID)
	assert.Equal(t, "sid-new", resp.StatusID)
}

// Ensure the package's mime/multipart writer references compile (helps catch
// import drift if someone removes the import after extracting helpers).
var _ = multipart.ErrMessageTooLarge
