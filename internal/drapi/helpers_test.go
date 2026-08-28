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

func TestURLMatchesConfiguredBase(t *testing.T) {
	seedBaseURL(t, "https://example.test/api/v2")

	for _, tc := range []struct {
		name string
		url  string
		want bool
	}{
		{name: "same scheme and host", url: "https://example.test/api/v2/files/?limit=1", want: true},
		{name: "same origin different API path", url: "https://example.test/api/v1/legacy/", want: true},
		{name: "different host", url: "https://other.example.test/api/v2/files/", want: false},
		{name: "different scheme", url: "http://example.test/api/v2/files/", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := URLMatchesConfiguredBase(tc.url)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestURLMatchesConfiguredBase_RejectsAmbiguousTargets(t *testing.T) {
	seedBaseURL(t, "https://example.test")

	for _, rawURL := range []string{"None/api/v2/files/", "://bad"} {
		t.Run(rawURL, func(t *testing.T) {
			got, err := URLMatchesConfiguredBase(rawURL)

			require.Error(t, err)
			assert.False(t, got)
		})
	}
}

func TestURLMatchesConfiguredBase_RequiresConfiguredBase(t *testing.T) {
	seedBaseURL(t, "")

	got, err := URLMatchesConfiguredBase("https://example.test/api/v2/files/")

	require.Error(t, err)
	assert.False(t, got)
}

func TestAssertNextOnSameHost(t *testing.T) {
	seedBaseURL(t, "https://example.test/api/v2")

	require.NoError(t, AssertNextOnSameHost("https://example.test/api/v2/files/?offset=100"))

	err := AssertNextOnSameHost("https://other.example.test/api/v2/files/?offset=100")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match API base host")
}

// ErrFromResp interprets nothing: drapi serves APIs that disagree on their
// error envelope, so even a well-formed FastAPI detail document stays a raw
// body= dump here. Reading meaning into it is a caller's job (see
// workload/apiclient).
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
	assert.Empty(t, httpErr.Detail, "drapi does not read the envelope")
	assert.JSONEq(t, `{"detail":"boom"}`, string(httpErr.Body),
		"the raw bytes ride the error so a caller that knows its API's envelope can")
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
