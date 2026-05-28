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
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringReadCloser wraps a string reader to satisfy http.Response.Body.
type stringReadCloser struct{ *strings.Reader }

func (s *stringReadCloser) Close() error { return nil }

func bodyOf(s string) io.ReadCloser {
	return &stringReadCloser{strings.NewReader(s)}
}

func TestExtractErrorDetail(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty body",
			body: "",
			want: "",
		},
		{
			name: "string detail",
			body: `{"detail": "lattice missing"}`,
			want: "lattice missing",
		},
		{
			name: "object detail",
			body: `{"detail": {"field": "name", "msg": "required"}}`,
			want: `{"field":"name","msg":"required"}`,
		},
		{
			name: "no detail field falls back to raw body",
			body: `{"unrelated": "value"}`,
			want: `{"unrelated": "value"}`,
		},
		{
			name: "malformed JSON falls back to raw body",
			body: "not json",
			want: "not json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractErrorDetail([]byte(tc.body))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeHTTPError_WithDetail(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       bodyOf(`{"detail": "lattice missing"}`),
	}

	err := decodeHTTPError(resp, "http://example/api/v2/pipelines")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
	assert.Contains(t, err.Error(), "lattice missing")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "detail response must still be *drapi.HTTPError so errors.As works")
	assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
	assert.Equal(t, "lattice missing", httpErr.Detail)
}

func TestDecodeHTTPError_WithoutDetail_ReturnsHTTPError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       bodyOf(""),
	}

	err := decodeHTTPError(resp, "http://example/api/v2/pipelines/abc")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Equal(t, "http://example/api/v2/pipelines/abc", httpErr.URL)
}

func TestBuildMultipartBody_IncludesFileAndFields(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "pipeline.py")

	const content = "from covalent import lattice\n"

	require.NoError(t, writeFile(filePath, content))

	fields := map[string]string{
		"description": "draft 1",
		"mode":        "draft",
	}

	body, contentType, err := buildMultipartBody(filePath, fields)
	require.NoError(t, err)
	assert.NotEmpty(t, body.Bytes())

	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	require.NotEmpty(t, params["boundary"])

	reader := multipart.NewReader(body, params["boundary"])

	seen := map[string]string{}

	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}

		require.NoError(t, partErr)

		buf, readErr := io.ReadAll(part)
		require.NoError(t, readErr)

		name := part.FormName()
		if name == "file" {
			assert.Equal(t, "pipeline.py", part.FileName())
		}

		seen[name] = string(buf)
	}

	assert.Equal(t, content, seen["file"])
	assert.Equal(t, "draft 1", seen["description"])
	assert.Equal(t, "draft", seen["mode"])
}

func TestBuildMultipartBody_MissingFile(t *testing.T) {
	_, _, err := buildMultipartBody("/no/such/file.py", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open /no/such/file.py")
}

// writeFile is a tiny helper that avoids dragging os into every test.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
