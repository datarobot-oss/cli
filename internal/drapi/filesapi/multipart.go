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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/datarobot/cli/internal/drapi"
)

// multipartFormField is the form field name every upload uses.
const multipartFormField = "file"

// newStreamingMultipartRequest builds a multipart POST whose body is
// piped through an io.Pipe rather than buffered. Memory cost is bounded
// by the pipe (one chunk in flight) plus the small envelope, regardless
// of file size — important because the engine may upload multi-GiB zips.
//
// Trade-off: the request has no GetBody, so http.Transport cannot
// transparently retry the body on connection reset. Callers needing
// retry must redo the call from scratch (re-opening the source if it
// isn't seekable).
func newStreamingMultipartRequest(
	requestURL string,
	query url.Values,
	filename string,
	size int64,
	body io.Reader,
) (*http.Request, error) {
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	contentType, prologue, epilogue, err := multipartFraming(filename)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()

	go streamMultipartBody(pw, prologue, body, epilogue)

	req, err := http.NewRequest(http.MethodPost, requestURL, pr)
	if err != nil {
		_ = pr.Close()

		return nil, fmt.Errorf("build multipart request: %w", err)
	}

	if size >= 0 {
		req.ContentLength = int64(len(prologue)) + size + int64(len(epilogue))
	}

	if err := drapi.AuthorizeRequest(req); err != nil {
		_ = pr.Close()

		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	return req, nil
}

// multipartFraming returns the prologue and epilogue around a single
// file part. Going through multipart.Writer keeps the framing
// RFC-2046-correct even though we stream the body separately.
func multipartFraming(filename string) (string, []byte, []byte, error) {
	var head bytes.Buffer

	w := multipart.NewWriter(&head)

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, multipartFormField, filename))
	hdr.Set("Content-Type", "application/octet-stream")

	if _, err := w.CreatePart(hdr); err != nil {
		return "", nil, nil, fmt.Errorf("create multipart part: %w", err)
	}

	contentType := w.FormDataContentType()
	headEnd := head.Len()

	if err := w.Close(); err != nil {
		return "", nil, nil, fmt.Errorf("close multipart writer: %w", err)
	}

	buf := head.Bytes()

	return contentType, buf[:headEnd], buf[headEnd:], nil
}

// streamMultipartBody surfaces body-read errors via CloseWithError so
// client.Do returns a body-read failure instead of hanging.
func streamMultipartBody(pw *io.PipeWriter, prologue []byte, body io.Reader, epilogue []byte) {
	defer pw.Close()

	if _, err := pw.Write(prologue); err != nil {
		_ = pw.CloseWithError(err)

		return
	}

	if _, err := io.Copy(pw, body); err != nil {
		_ = pw.CloseWithError(fmt.Errorf("stream upload body: %w", err))

		return
	}

	if _, err := pw.Write(epilogue); err != nil {
		_ = pw.CloseWithError(err)

		return
	}
}
