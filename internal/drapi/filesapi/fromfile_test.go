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
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multipartPart is one decoded part of a multipart request body, in wire
// order. FileName is non-empty for file parts.
type multipartPart struct {
	Name     string
	FileName string
	Content  []byte
}

// decodeMultipart walks a multipart reader and returns its parts in wire
// order. Form fields written before the file part appear before it here,
// which is how tests pin the fields-first convention.
func decodeMultipart(mr *multipart.Reader) ([]multipartPart, error) {
	var parts []multipartPart

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return parts, nil
		}

		if err != nil {
			return nil, err
		}

		content, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}

		parts = append(parts, multipartPart{
			Name:     part.FormName(),
			FileName: part.FileName(),
			Content:  content,
		})
	}
}

// readMultipartParts parses an UNCONSUMED multipart request body with
// mime/multipart and returns its parts in wire order. Tests assert on
// these decoded parts rather than raw bytes so the framing stays free to
// evolve. Reading r.Body first (e.g. for a Content-Length check) exhausts
// the stream — parse the buffered copy via multipart.NewReader instead.
func readMultipartParts(r *http.Request) ([]multipartPart, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}

	return decodeMultipart(mr)
}

// TestUploadFromZipExisting_UseArchiveContentsStaysInQuery pins the
// placement of useArchiveContents. The monorepo fromFile validator
// declares it as a multipart form field whose default is 'True' (archive
// extraction), and the route binds validator fields from the parsed form
// only — request.args is never consulted. Extraction therefore happens via
// the server default regardless of what the CLI sends, so leaving the
// flag in the query string (where it is ignored) changes nothing
// observable. This test documents that decision so any future move into
// the form is a conscious one.
func TestUploadFromZipExisting_UseArchiveContentsStaysInQuery(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "true", r.URL.Query().Get("useArchiveContents"))

		parts, err := readMultipartParts(r)
		if !assert.NoError(t, err) {
			return
		}

		for _, part := range parts {
			assert.NotEqual(t, "useArchiveContents", part.Name,
				"useArchiveContents is intentionally left out of the form body")
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v9","statusId":"sid-9"}`))
	}))

	c := New()

	body := bytes.NewReader([]byte("PK\x03\x04fake-zip"))
	_, err := c.UploadFromZipExisting("cid-1", "changes.zip", OverwriteReplace, int64(body.Len()), body)
	require.NoError(t, err)
}

// TestUploadFromZipNew_NoOverwriteField guards the shared-framing parity:
// a first sync has no pre-existing paths, so it must send no overwrite
// form field and no overwrite query parameter — the server's RENAME
// default is correct there and the request should stay minimal.
func TestUploadFromZipNew_NoOverwriteField(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("overwrite"))

		parts, err := readMultipartParts(r)
		if !assert.NoError(t, err) {
			return
		}

		for _, part := range parts {
			assert.NotEqual(t, "overwrite", part.Name)
		}

		if !assert.Len(t, parts, 1) {
			return
		}

		assert.Equal(t, "file", parts[0].Name)
		assert.Equal(t, "wapi-sync.zip", parts[0].FileName)

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"new-cid","catalogVersionId":"new-ver","statusId":"sid-new"}`))
	}))

	c := New()

	body := bytes.NewReader([]byte("PK\x03\x04fake-zip"))
	resp, err := c.UploadFromZipNew("wapi-sync.zip", int64(body.Len()), body)
	require.NoError(t, err)
	assert.Equal(t, "new-ver", resp.CatalogVersionID)
}

// TestUploadFromZipExisting_ContentLengthMatchesBodyWithFormFields
// verifies the Content-Length accounting survives folding form fields
// into the prologue: the advertised length must equal the received body
// exactly, and the streamed file must still decode intact. An off-by-N
// here surfaces as a transport error or a truncated multipart stream,
// never as a silent pass.
func TestUploadFromZipExisting_ContentLengthMatchesBodyWithFormFields(t *testing.T) {
	// Long enough to cross io.Copy's internal buffer more than once.
	payload := strings.Repeat("0123456789abcdef", 4096)

	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, r.ContentLength, int64(len(raw)),
			"advertised Content-Length must match the received body byte-for-byte")

		// The body is consumed by the ReadAll above, so parse the buffered
		// copy rather than asking the request for a fresh MultipartReader.
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !assert.NoError(t, err) {
			return
		}

		parts, err := decodeMultipart(multipart.NewReader(bytes.NewReader(raw), params["boundary"]))
		if !assert.NoError(t, err) {
			return
		}

		if !assert.Len(t, parts, 2) {
			return
		}

		assert.Equal(t, "REPLACE", string(parts[0].Content))
		assert.Equal(t, payload, string(parts[1].Content))

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v9","statusId":"sid-9"}`))
	}))

	c := New()

	body := bytes.NewReader([]byte(payload))
	_, err := c.UploadFromZipExisting("cid-1", "changes.zip", OverwriteReplace, int64(body.Len()), body)
	require.NoError(t, err)
}

// The package's mime/multipart types stay referenced even if helper
// implementations drift; mirrors the guard at the bottom of client_test.go.
var _ = multipart.ErrMessageTooLarge
