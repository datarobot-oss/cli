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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

// testMeta is metadata pointing at a stub token endpoint.
func testMeta(tokenURL string) *OAuthMetadata {
	return &OAuthMetadata{
		AuthorizationEndpoint: "https://as.example/oauth2/auth",
		TokenEndpoint:         tokenURL,
	}
}

// newTestOAuthFlow binds an OAuth flow on an ephemeral port against meta.
func newTestOAuthFlow(t *testing.T, meta *OAuthMetadata) *BrowserFlow {
	t.Helper()

	flow, err := newBrowserFlowOAuthOn("127.0.0.1:0", meta)
	require.NoError(t, err)

	t.Cleanup(func() { _ = flow.Close() })

	return flow
}

// tokenServer stands in for an authorization server's token endpoint.
func tokenServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	return srv
}

// Strict on purpose: a single-page app answers 200 with HTML for unknown paths,
// which would otherwise look like a successful discovery.
func TestDiscoverOAuth(t *testing.T) {
	const valid = `{"issuer":"https://as.example","authorization_endpoint":"https://as.example/oauth2/auth","token_endpoint":"https://as.example/oauth2/token"}`

	tests := []struct {
		name, contentType, body string
		wantOK                  bool
	}{
		{"HTML catch-all", "text/html", "<!doctype html><html>app shell</html>", false},
		{"missing token_endpoint", "application/json", `{"authorization_endpoint":"https://as.example/auth"}`, false},
		{"complete document", "application/json", valid, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			meta, err := DiscoverOAuth(context.Background(), srv.URL)

			if tc.wantOK {
				require.NoError(t, err)
				assert.Equal(t, "https://as.example/oauth2/token", meta.TokenEndpoint)

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrOAuthNotSupported)
		})
	}
}

// The authorize URL must carry PKCE and state, and name the bound port.
func TestOAuthFlow_AuthURLCarriesPKCEAndState(t *testing.T) {
	flow := newTestOAuthFlow(t, testMeta("https://as.example/oauth2/token"))

	u, err := url.Parse(flow.AuthURL())
	require.NoError(t, err)

	q := u.Query()
	assert.Equal(t, OAuthClientID, q.Get("client_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))
	assert.NotEmpty(t, q.Get("state"))
	assert.Equal(t, "http://"+flow.localAddr()+"/", q.Get("redirect_uri"))

	// The verifier is the secret half and must never leave the process.
	assert.NotContains(t, flow.AuthURL(), flow.oauthPKCE.verifier)
}

// Regression: listener.Addr() resolves "localhost" to 127.0.0.1, and
// authorization servers compare redirect_uri as an exact string. A server that
// registered http://localhost:51164/ rejects the IP literal outright, before
// the user ever sees a login page. The
// hostname must come from the configured address, the port from the listener.
func TestOAuthFlow_RedirectURIKeepsConfiguredHostname(t *testing.T) {
	flow, err := newBrowserFlowOAuthOn("localhost:0", testMeta("https://as.example/oauth2/token"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = flow.Close() })

	assert.Contains(t, flow.redirectURI, "http://localhost:",
		"redirect_uri must keep the configured hostname, not the resolved IP")
	assert.NotContains(t, flow.redirectURI, "127.0.0.1")

	// And it must still name the port actually bound, not the requested :0.
	_, boundPort, err := net.SplitHostPort(flow.localAddr())
	require.NoError(t, err)
	assert.Contains(t, flow.redirectURI, boundPort)
	assert.NotContains(t, flow.redirectURI, ":0/")
}

func TestOAuthFlow_ExchangesCodeForToken(t *testing.T) {
	srv := tokenServer(t, http.StatusOK, `{"access_token":"issued-token","token_type":"bearer","refresh_token":"r"}`)

	flow := newTestOAuthFlow(t, testMeta(srv.URL))

	token, err := driveFlow(t, flow, "?code=abc&state="+flow.oauthPKCE.state)

	require.NoError(t, err)
	assert.Equal(t, "issued-token", token, "Wait must return the exchanged access token, not the code")
}

// The listener is open to any local process; state is the only thing
// distinguishing a callback this login started.
func TestOAuthFlow_RejectsStateMismatch(t *testing.T) {
	srv := tokenServer(t, http.StatusOK, `{"access_token":"should-never-be-used"}`)

	flow := newTestOAuthFlow(t, testMeta(srv.URL))

	token, err := driveFlow(t, flow, "?code=abc&state=not-the-right-state")

	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "state mismatch")
	assert.NotErrorIs(t, err, ErrLoginInterrupted,
		"a rejected callback is a failure, not the port-reclaim interrupt")
}

// driveFlow fires the callback and returns Wait's result.
func driveFlow(t *testing.T, flow *BrowserFlow, query string) (string, error) {
	t.Helper()

	go func() {
		resp, err := http.Get(callbackURL(t, flow, query))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	return flow.Wait(context.Background())
}

// THE regression that matters: an empty `key` is the port-reclaim sentinel, so
// the OAuth branch must be evaluated first or every callback aborts the login.
func TestOAuthFlow_CodeCallbackIsNotSwallowedBySentinel(t *testing.T) {
	srv := tokenServer(t, http.StatusOK, `{"access_token":"survived"}`)

	flow := newTestOAuthFlow(t, testMeta(srv.URL))

	// No `key` parameter at all — exactly the shape the sentinel looks for.
	token, err := driveFlow(t, flow, "?code=abc&state="+flow.oauthPKCE.state)

	require.NoError(t, err, "an OAuth callback must not be read as the interrupt sentinel")
	assert.Equal(t, "survived", token)
}

// Gate off must not reach out at all — this protects hosts that serve a
// discovery document without supporting the flow.
func TestNewBrowserFlowContext_GateOffIssuesNoDiscovery(t *testing.T) {
	t.Setenv(OAuthEnabledEnv, "")

	probed := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probed = true

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	flow, err := NewBrowserFlowContext(context.Background(), srv.URL, nil)
	require.NoError(t, err)

	defer func() { _ = flow.Close() }()

	assert.False(t, probed, "no request may be made to the endpoint when the gate is off")
	assert.Nil(t, flow.oauthMeta, "gate off must produce a legacy flow")
	assert.Contains(t, flow.AuthURL(), "/account/developer-tools?cliRedirect=true")
}

// Gate on but undiscoverable must fail loudly, not downgrade silently.
func TestNewBrowserFlowContext_GateOnWithoutDiscoveryFails(t *testing.T) {
	t.Setenv(OAuthEnabledEnv, "true")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := NewBrowserFlowContext(context.Background(), srv.URL, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOAuthNotSupported)
}

// refreshServer is a token endpoint that records the form it received.
type refreshServer struct {
	*httptest.Server
	gotForm url.Values
}

func newRefreshServer(t *testing.T, status int, body string) *refreshServer {
	t.Helper()

	rs := &refreshServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
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
		{"nil", nil, false},
		{"401 judged", &config.HTTPStatusError{StatusCode: 401}, true},
		{"403 judged", &config.HTTPStatusError{StatusCode: 403}, true},
		{"5xx unjudged", &config.HTTPStatusError{StatusCode: 503}, false},
		{"transport error unjudged", assert.AnError, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tokenWasRejected(tc.err))
		})
	}
}
