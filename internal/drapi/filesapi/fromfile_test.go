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
	"mime/multipart"
	"net/http"
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

// TestUploadFromZipExisting_UseArchiveContentsTravelsInTheForm pins the
// placement of useArchiveContents. The monorepo fromFile validator declares
// it as a multipart form field whose default is 'True' (archive extraction),
// and the route binds validator fields from the parsed form only —
// request.args is never consulted. The flag used to be sent in the query,
// where it changed nothing observable and stayed correct only for as long as
// the server default did; a default flip, or the Files API starting to reject
// recognised parameters sent in the query, would have turned every zip upload
// into a single archive file or a failure with no CLI change. It travels in
// the form now, before the file part, and not in the query at all.
func TestUploadFromZipExisting_UseArchiveContentsTravelsInTheForm(t *testing.T) {
	startServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.False(t, r.URL.Query().Has("useArchiveContents"),
			"the query is not a channel the server reads, so nothing should be stated there")

		parts, err := readMultipartParts(r)
		if !assert.NoError(t, err) {
			return
		}

		var flag *multipartPart

		fileAt := -1

		for i := range parts {
			switch parts[i].Name {
			case "useArchiveContents":
				flag = &parts[i]
			case multipartFormField:
				fileAt = i
			}
		}

		if assert.NotNil(t, flag, "useArchiveContents must be a form field") {
			assert.Equal(t, "true", string(flag.Content))
			assert.Empty(t, flag.FileName)
		}

		assert.Equal(t, len(parts)-1, fileAt, "the file part comes last, after every field")

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"catalogId":"cid-1","catalogVersionId":"v9","statusId":"sid-9"}`))
	}))

	c := New()

	body := bytes.NewReader([]byte("PK\x03\x04fake-zip"))
	_, err := c.UploadFromZipExisting("cid-1", "changes.zip", OverwriteReplace, int64(body.Len()), body)
	require.NoError(t, err)
}

// The package's mime/multipart types stay referenced even if helper
// implementations drift; mirrors the guard at the bottom of client_test.go.
var _ = multipart.ErrMessageTooLarge
