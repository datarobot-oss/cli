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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetTokenCache clears the package-level token cache so tests don't leak
// state into each other. The `token` variable is defined in get.go.
func resetTokenCache(t *testing.T) {
	t.Helper()

	prev := token

	token = ""

	t.Cleanup(func() {
		token = prev
	})
}

// withSkipAuth installs a deterministic token in viper and turns on
// --skip-auth so resolveToken returns immediately without hitting the
// network. Returns the token that was installed for assertion.
func withSkipAuth(t *testing.T, value string) string {
	t.Helper()

	prevSkip := viperx.GetBool(config.SkipAuthKey)
	prevKey := viperx.GetString(config.DataRobotAPIKey)

	viperx.Set(config.SkipAuthKey, true)
	viperx.Set(config.DataRobotAPIKey, value)

	t.Cleanup(func() {
		viperx.Set(config.SkipAuthKey, prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevKey)
	})

	return value
}

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{name: "positive timeout passes through", timeout: 7 * time.Second, want: 7 * time.Second},
		{name: "zero timeout clamps to default", timeout: 0, want: DefaultClientTimeout},
		{name: "negative timeout clamps to default", timeout: -5 * time.Second, want: DefaultClientTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHTTPClient(tt.timeout)
			require.NotNil(t, c)
			assert.Equal(t, tt.want, c.Timeout)
		})
	}
}

func TestDefaultClientTimeout(t *testing.T) {
	// Spot-check the constant — guards against accidental changes that
	// would slow down or speed up every API call.
	assert.Equal(t, 30*time.Second, DefaultClientTimeout)
}

func TestGetToken_ResolvesAndMemoizes(t *testing.T) {
	resetTokenCache(t)
	withSkipAuth(t, "abc123")

	got, err := getToken()
	require.NoError(t, err)
	assert.Equal(t, "abc123", got)

	// Mutating viper after the cache is populated should NOT change what
	// getToken returns — the value is memoized for the lifetime of the
	// process.
	viperx.Set(config.DataRobotAPIKey, "different")

	cached, err := getToken()
	require.NoError(t, err)
	assert.Equal(t, "abc123", cached)
}

func TestAuthorizeRequest_SetsExpectedHeaders(t *testing.T) {
	resetTokenCache(t)
	withSkipAuth(t, "shhh")

	req, err := http.NewRequest(http.MethodGet, "http://example/api/v2/foo", nil)
	require.NoError(t, err)

	require.NoError(t, AuthorizeRequest(req))

	assert.Equal(t, "Bearer shhh", req.Header.Get("Authorization"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
}

func TestAuthorizeRequest_DoesNotConsumeBody(t *testing.T) {
	resetTokenCache(t)
	withSkipAuth(t, "shhh")

	req, err := http.NewRequest(http.MethodPost, "http://example/api/v2/foo",
		io.NopCloser(strings.NewReader("payload")))
	require.NoError(t, err)

	require.NoError(t, AuthorizeRequest(req))

	// Body must still be readable after AuthorizeRequest — this is the
	// invariant that makes it safe for multipart uploads.
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(body))
}

// TestTokenAccessorsAreGuarded drives SetToken and GetToken against getToken
// concurrently. All three touch the same package global, and getToken is
// reached from two goroutines whenever GetLLMsAndDeployed runs, so a re-auth
// path calling SetToken mid-fetch must not race the readers. Only fails under
// -race, which is how the suite runs.
func TestTokenAccessorsAreGuarded(t *testing.T) {
	resetTokenCache(t)
	want := withSkipAuth(t, "concurrent-token")

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(3)

		go func() { defer wg.Done(); SetToken(want) }()
		go func() { defer wg.Done(); _ = GetToken() }()

		go func() {
			defer wg.Done()

			got, err := getToken()
			assert.NoError(t, err)
			assert.Equal(t, want, got)
		}()
	}

	wg.Wait()

	assert.Equal(t, want, GetToken())
}
