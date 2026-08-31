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
	"sort"

	"github.com/datarobot/cli/internal/drapi"
)

// multipartFormField is the form field name every upload uses.
const multipartFormField = "file"

// newStreamingMultipartRequest builds a multipart POST whose body is
// piped through an io.Pipe rather than buffered. Memory cost is bounded
// by the pipe (one chunk in flight) plus the small envelope, regardless
// of file size — important because the engine may upload multi-GiB zips.
//
// requestURL is used as-is: callers build any query parameters into it
// before calling. Fields ride in the multipart body, not the URL query:
// some server routes (fromFile) bind their validator fields from the
// parsed form only and silently ignore query parameters. Fields are
// framed as complete parts BEFORE the file part so a streaming parser
// collects them without buffering the file.
//
// Trade-off: the request has no GetBody, so http.Transport cannot
// transparently retry the body on connection reset. Callers needing
// retry must redo the call from scratch (re-opening the source if it
// isn't seekable).
func newStreamingMultipartRequest(
	requestURL string,
	fields url.Values,
	filename string,
	size int64,
	body io.Reader,
) (*http.Request, error) {
	contentType, prologue, epilogue, err := multipartFraming(fields, filename)
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

	// ContentLength stays exact because the form fields are folded into
	// the prologue; the file bytes still contribute exactly size, and the
	// epilogue is unchanged.
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

// multipartFraming returns the prologue and epilogue around the streamed
// file part, with any extra form fields framed as complete parts first.
// Fields must precede the file part: streaming parsers read form fields
// as they arrive, so a server can collect its parameters before
// committing to an arbitrarily large file stream. Field names are sorted
// so the framing is deterministic. Going through multipart.Writer keeps
// the framing RFC-2046-correct even though we stream the file content
// separately.
func multipartFraming(fields url.Values, filename string) (string, []byte, []byte, error) {
	var head bytes.Buffer

	w := multipart.NewWriter(&head)

	names := make([]string, 0, len(fields))

	for name := range fields {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		for _, value := range fields[name] {
			if err := w.WriteField(name, value); err != nil {
				return "", nil, nil, fmt.Errorf("write multipart field %s: %w", name, err)
			}
		}
	}

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
