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
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeRequest_AuthorizationAndUserAgent(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	req, err := http.NewRequest(http.MethodGet, "http://example/", nil)
	require.NoError(t, err)

	require.NoError(t, AuthorizeRequest(req))

	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
}

func TestAuthorizeRequest_TraceHeaderEnabled(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.APIConsumerTrackingEnabled, true)

	req, err := http.NewRequest(http.MethodGet, "http://example/", nil)
	require.NoError(t, err)

	require.NoError(t, AuthorizeRequest(req))

	assert.NotEmpty(t, req.Header.Get("X-DataRobot-Api-Consumer-Trace"))
}

func TestAuthorizeRequest_TraceHeaderDisabled(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	viperx.Reset()
	t.Cleanup(viperx.Reset)
	viperx.Set(config.APIConsumerTrackingEnabled, false)

	req, err := http.NewRequest(http.MethodGet, "http://example/", nil)
	require.NoError(t, err)

	require.NoError(t, AuthorizeRequest(req))

	assert.Empty(t, req.Header.Get("X-DataRobot-Api-Consumer-Trace"))
}

// TestAuthorizeRequest_MemoizesToken confirms a seeded token short-circuits
// config.GetAPIKey on subsequent calls — the second AuthorizeRequest call
// must succeed without contacting config (which would fail in this test env).
func TestAuthorizeRequest_MemoizesToken(t *testing.T) {
	defer resetTokenForTest(t, "test-token")()

	for range 2 {
		req, err := http.NewRequest(http.MethodGet, "http://example/", nil)
		require.NoError(t, err)

		require.NoError(t, AuthorizeRequest(req))
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
	}
}

// TestAuthorizeRequest_PropagatesTokenError confirms that when the token is
// unset and config.GetAPIKey fails (no DR_API_TOKEN, no config file in the
// test env), the error is returned rather than silently producing a request
// with an empty bearer.
func TestAuthorizeRequest_PropagatesTokenError(t *testing.T) {
	defer resetTokenForTest(t, "")()

	viperx.Reset()
	t.Cleanup(viperx.Reset)

	req, err := http.NewRequest(http.MethodGet, "http://example/", nil)
	require.NoError(t, err)

	err = AuthorizeRequest(req)
	require.Error(t, err)
}
