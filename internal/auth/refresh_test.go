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

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

// refreshServer records the form it was sent and answers with the given status
// and body.
type refreshServer struct {
	*httptest.Server
	gotForm url.Values
}

func newRefreshServer(t *testing.T, status int, body string) *refreshServer {
	t.Helper()

	rs := &refreshServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			// Not require/assert: this runs on the server goroutine, where a
			// failed assertion cannot fail the test cleanly. Surface it as a
			// response the test will notice instead.
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		rs.gotForm = r.PostForm

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(rs.Close)

	return rs
}

// A profile authenticated by the legacy hand-off has nothing to renew with.
// That is not a failure — it means "go and do an interactive login".
func TestRefreshAccessToken_NoStoredMaterial(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	ClearOAuthState()

	_, err := RefreshAccessToken(context.Background())

	assert.ErrorIs(t, err, ErrNoRefreshToken)
}

func TestRefreshAccessToken_Success(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	rs := newRefreshServer(t, http.StatusOK, `{"access_token":"renewed","token_type":"bearer"}`)
	StoreOAuthState("stored-refresh", rs.URL)

	token, err := RefreshAccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "renewed", token)
	assert.Equal(t, "renewed", viperx.GetString(config.DataRobotAPIKey),
		"the renewed token must land where the API layer reads it")

	assert.Equal(t, "refresh_token", rs.gotForm.Get("grant_type"))
	assert.Equal(t, "stored-refresh", rs.gotForm.Get("refresh_token"))
	assert.Equal(t, OAuthClientID, rs.gotForm.Get("client_id"))

	// Not rotated by this server, so the stored one must still be there.
	assert.Equal(t, "stored-refresh", viperx.GetString(config.OAuthRefreshToken))
}

// Servers that rotate refresh tokens invalidate the old one on use, so a
// returned value has to replace what is stored or the NEXT renewal fails.
func TestRefreshAccessToken_StoresRotatedRefreshToken(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	rs := newRefreshServer(t, http.StatusOK,
		`{"access_token":"renewed","refresh_token":"rotated"}`)
	StoreOAuthState("stored-refresh", rs.URL)

	_, err := RefreshAccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "rotated", viperx.GetString(config.OAuthRefreshToken),
		"a rotated refresh token must replace the spent one")
}

// A rejected refresh token is spent. Keeping it would make every subsequent
// command retry something that cannot work before falling back.
func TestRefreshAccessToken_RejectionClearsState(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	rs := newRefreshServer(t, http.StatusBadRequest,
		`{"error":"invalid_grant","error_description":"token is expired"}`)
	StoreOAuthState("stored-refresh", rs.URL)

	_, err := RefreshAccessToken(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_grant")
	assert.Contains(t, err.Error(), "token is expired", "the description is the actionable half")
	assert.Empty(t, viperx.GetString(config.OAuthRefreshToken),
		"a spent refresh token must not be kept")
	assert.Empty(t, viperx.GetString(config.OAuthTokenEndpoint))
}

// THE guard. Only a judged rejection means the token expired. Treating a 404,
// 429, 5xx or transport error as expiry would spend a refresh token — possibly
// a rotate-on-use one — against a server that never rejected anything, turning
// a transient outage into a forced re-login.
func TestTokenWasRejected(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not a rejection", nil, false},
		{"401 is a rejection", &config.HTTPStatusError{StatusCode: 401}, true},
		{"403 is a rejection", &config.HTTPStatusError{StatusCode: 403}, true},
		{"404 is unjudged", &config.HTTPStatusError{StatusCode: 404}, false},
		{"429 is unjudged", &config.HTTPStatusError{StatusCode: 429}, false},
		{"500 is unjudged", &config.HTTPStatusError{StatusCode: 500}, false},
		{"503 is unjudged", &config.HTTPStatusError{StatusCode: 503}, false},
		{"a transport error is unjudged", assert.AnError, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tokenWasRejected(tc.err))
		})
	}
}

// Clearing the access token alone would leave a working refresh token on disk.
func TestClearOAuthState(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	StoreOAuthState("stored-refresh", "https://as.example/oauth2/token")
	require.NotEmpty(t, viperx.GetString(config.OAuthRefreshToken))

	ClearOAuthState()

	assert.Empty(t, viperx.GetString(config.OAuthRefreshToken))
	assert.Empty(t, viperx.GetString(config.OAuthTokenEndpoint))
}

// Both keys must be on the persist allowlist, or a renewal survives only until
// the process exits and every new command starts with a browser.
func TestOAuthKeysArePersistable(t *testing.T) {
	for _, key := range []string{config.OAuthRefreshToken, config.OAuthTokenEndpoint} {
		_, ok := config.PersistableKeys[key]
		assert.True(t, ok, "%s must be persistable or renewal state never reaches disk", key)
	}
}
