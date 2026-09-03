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
)

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

func TestOAuthRequested(t *testing.T) {
	yes, no := true, false

	tests := []struct {
		name     string
		env      string
		override *bool
		want     bool
	}{
		{"unset is off — the whole point of the gate", "", nil, false},
		{"env true", "true", nil, true},
		{"env 1", "1", nil, true},
		{"env garbage is off, not an error", "banana", nil, false},
		{"flag beats unset env", "", &yes, true},
		{"flag --no-oauth beats env true", "true", &no, false},
		{"flag --oauth beats env absent", "", &yes, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(OAuthEnabledEnv, tc.env)
			assert.Equal(t, tc.want, OAuthRequested(tc.override))
		})
	}
}

// A single-page app answers 200 with its HTML shell for unknown paths. Treating
// that as a successful discovery would send the CLI into an OAuth flow against
// an endpoint that cannot complete one.
func TestDiscoverOAuth_RejectsHTMLCatchAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body>app shell</body></html>"))
	}))
	defer srv.Close()

	_, err := DiscoverOAuth(context.Background(), srv.URL)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOAuthNotSupported)
}

func TestDiscoverOAuth_RejectsIncompleteDocument(t *testing.T) {
	// Valid JSON, but missing token_endpoint — unusable.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"http://x","authorization_endpoint":"http://x/auth"}`))
	}))
	defer srv.Close()

	_, err := DiscoverOAuth(context.Background(), srv.URL)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOAuthNotSupported)
}

func TestDiscoverOAuth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, discoveryPath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issuer":"http://hydra",
			"authorization_endpoint":"http://hydra/oauth2/auth",
			"token_endpoint":"http://hydra/oauth2/token"
		}`))
	}))
	defer srv.Close()

	meta, err := DiscoverOAuth(context.Background(), srv.URL)

	require.NoError(t, err)
	assert.Equal(t, "http://hydra/oauth2/auth", meta.AuthorizationEndpoint)
	assert.Equal(t, "http://hydra/oauth2/token", meta.TokenEndpoint)
}

// The authorize URL must carry PKCE and state, and the redirect URI must name
// the port actually bound or the code comes back to nothing.
func TestOAuthFlow_AuthURLCarriesPKCEAndState(t *testing.T) {
	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         "http://hydra/oauth2/token",
	})

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
// authorization servers compare redirect_uri as an exact string. Hydra
// registers http://localhost:51164/, so emitting the IP literal gets the
// authorize request rejected before the user ever sees a login page. The
// hostname must come from the configured address, the port from the listener.
func TestOAuthFlow_RedirectURIKeepsConfiguredHostname(t *testing.T) {
	flow, err := newBrowserFlowOAuthOn("localhost:0", &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         "http://hydra/oauth2/token",
	})
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
	srv := tokenServer(t, http.StatusOK, `{"access_token":"hydra-token","token_type":"bearer","refresh_token":"r"}`)

	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         srv.URL,
	})

	go func() {
		resp, err := http.Get(callbackURL(t, flow, "?code=abc&state="+flow.oauthPKCE.state))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	token, err := flow.Wait(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "hydra-token", token, "Wait must return the exchanged access token, not the code")
}

// A callback this login did not start must be refused: the listener is open to
// any local process, and state is the only thing distinguishing them.
func TestOAuthFlow_RejectsStateMismatch(t *testing.T) {
	srv := tokenServer(t, http.StatusOK, `{"access_token":"should-never-be-used"}`)

	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         srv.URL,
	})

	go func() {
		resp, err := http.Get(callbackURL(t, flow, "?code=abc&state=not-the-right-state"))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	token, err := flow.Wait(context.Background())

	require.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "state mismatch")
	assert.NotErrorIs(t, err, ErrLoginInterrupted,
		"a rejected callback is a failure, not the port-reclaim interrupt")
}

func TestOAuthFlow_SurfacesTokenEndpointRejection(t *testing.T) {
	srv := tokenServer(t, http.StatusBadRequest,
		`{"error":"invalid_grant","error_description":"code already used"}`)

	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         srv.URL,
	})

	go func() {
		resp, err := http.Get(callbackURL(t, flow, "?code=abc&state="+flow.oauthPKCE.state))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	_, err := flow.Wait(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_grant")
	assert.Contains(t, err.Error(), "code already used", "the description is the actionable half")
}

// The authorization server can report failure on the redirect itself, e.g. the
// user declining consent. That must not wait out the five-minute timeout.
func TestOAuthFlow_SurfacesAuthorizationError(t *testing.T) {
	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         "http://hydra/oauth2/token",
	})

	go func() {
		resp, err := http.Get(callbackURL(t, flow, "?error=access_denied&error_description=user+declined"))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	_, err := flow.Wait(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_denied")
	assert.NotErrorIs(t, err, ErrLoginInterrupted)
}

// THE regression that matters. An empty `key` is the port-reclaim sentinel, so
// the OAuth branch has to be evaluated first — otherwise every OAuth callback
// reads as "another CLI wants this port" and aborts the login.
func TestOAuthFlow_CodeCallbackIsNotSwallowedBySentinel(t *testing.T) {
	srv := tokenServer(t, http.StatusOK, `{"access_token":"survived"}`)

	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         srv.URL,
	})

	// No `key` parameter at all — exactly the shape the sentinel looks for.
	go func() {
		resp, err := http.Get(callbackURL(t, flow, "?code=abc&state="+flow.oauthPKCE.state))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	token, err := flow.Wait(context.Background())

	require.NoError(t, err, "an OAuth callback must not be read as the interrupt sentinel")
	assert.Equal(t, "survived", token)
}

// And the sentinel itself must still work in OAuth mode, or a stale process can
// never be asked to release the port.
func TestOAuthFlow_KeylessCallbackStillInterrupts(t *testing.T) {
	flow := newTestOAuthFlow(t, &OAuthMetadata{
		AuthorizationEndpoint: "http://hydra/oauth2/auth",
		TokenEndpoint:         "http://hydra/oauth2/token",
	})

	go func() {
		resp, err := http.Get(callbackURL(t, flow, ""))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	_, err := flow.Wait(context.Background())

	assert.ErrorIs(t, err, ErrLoginInterrupted)
}

// With the gate off, the constructor must not reach out at all — this is what
// protects deployments that serve a discovery document with no login service.
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

// Gate on but discovery unusable must fail loudly. A silent downgrade to the
// legacy flow would look like success while producing a different credential.
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
