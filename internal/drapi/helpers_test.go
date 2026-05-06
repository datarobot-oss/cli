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

package drapi

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedBaseURL(t *testing.T, base string) {
	t.Helper()

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.DataRobotURL, base)
}

func TestEndpointURL_NoQuery(t *testing.T) {
	seedBaseURL(t, "https://example.test")

	got, err := EndpointURL("/files/", nil)
	require.NoError(t, err)
	assert.Equal(t, "https://example.test/api/v2/files/", got)
}

func TestEndpointURL_WithQuery(t *testing.T) {
	seedBaseURL(t, "https://example.test")

	q := url.Values{}
	q.Set("limit", "100")
	q.Set("offset", "50")

	got, err := EndpointURL("/files/", q)
	require.NoError(t, err)
	// url.Values encodes deterministically (alphabetical key order).
	assert.Equal(t, "https://example.test/api/v2/files/?limit=100&offset=50", got)
}

func TestEndpointURL_PropagatesConfigError(t *testing.T) {
	seedBaseURL(t, "")

	got, err := EndpointURL("/files/", nil)
	require.Error(t, err)
	assert.Empty(t, got)
}

func TestErrFromResp_WithBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(`{"detail":"boom"}`)),
	}

	err := ErrFromResp(resp, "https://example.test/api/v2/files/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `body={"detail":"boom"}`)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}

func TestErrFromResp_EmptyBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}

	err := ErrFromResp(resp, "https://example.test/api/v2/files/abc/")
	require.Error(t, err)

	var httpErr *HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.NotContains(t, err.Error(), "body=")
}

// TestErrFromResp_TruncatesLargeBody confirms the 512-byte cap is honored
// so a hostile or runaway response body cannot blow up an error message.
func TestErrFromResp_TruncatesLargeBody(t *testing.T) {
	body := strings.Repeat("a", 4096)

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := ErrFromResp(resp, "https://example.test/api/v2/files/")
	require.Error(t, err)

	const maxBodyBytes = 512

	idx := strings.Index(err.Error(), "body=")
	require.GreaterOrEqual(t, idx, 0)

	captured := err.Error()[idx+len("body="):]
	assert.Len(t, captured, maxBodyBytes)
}
