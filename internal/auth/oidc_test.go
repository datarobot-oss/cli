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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIDP is a minimal OIDC provider: discovery, an authorize endpoint the test
// never visits (the stubbed browser plays the user), and a token endpoint that
// checks the PKCE verifier against the challenge it was sent.
type fakeIDP struct {
	server        *httptest.Server
	issuer        string // overrides the discovery document's issuer when set
	challenge     string
	tokenRequests int
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()

	idp := &fakeIDP{}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		issuer := idp.server.URL
		if idp.issuer != "" {
			issuer = idp.issuer
		}

		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 issuer,
			"authorization_endpoint": idp.server.URL + "/authorize",
			"token_endpoint":         idp.server.URL + "/token",
		})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idp.tokenRequests++

		_ = r.ParseForm()

		w.Header().Set("Content-Type", "application/json")

		if r.Form.Get("code") != "good-code" || pkceChallenge(r.Form.Get("code_verifier")) != idp.challenge {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "idp-access-token", "token_type": "Bearer", "expires_in": 3600})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)

	return idp
}

// freeLoopbackRedirect returns a redirect URI on a port nothing is listening on.
func freeLoopbackRedirect(t *testing.T, path string) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

// stubBrowser replaces the browser with a function that plays the user: it
// checks the authorize URL, then follows the redirect the IdP would send.
func stubBrowser(t *testing.T, idp *fakeIDP, callbackQuery func(state string) url.Values) {
	t.Helper()

	orig := openBrowser

	t.Cleanup(func() { openBrowser = orig })

	openBrowser = func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}

		q := u.Query()

		assert.Equal(t, "test-client", q.Get("client_id"))
		assert.Equal(t, "code", q.Get("response_type"))
		assert.Equal(t, "S256", q.Get("code_challenge_method"))
		assert.Equal(t, "openid profile", q.Get("scope"))

		idp.challenge = q.Get("code_challenge")

		redirect, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			return err
		}

		redirect.RawQuery = callbackQuery(q.Get("state")).Encode()

		go func() {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, redirect.String(), nil)
			req.Header.Set("Sec-Fetch-Dest", "document")

			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()

		return nil
	}
}

func testOIDCConfig(idp *fakeIDP, redirectURI string) OIDCConfig {
	return OIDCConfig{Issuer: idp.server.URL, ClientID: "test-client", RedirectURI: redirectURI}
}

func TestRunOIDCLogin_ReturnsAccessToken(t *testing.T) {
	idp := newFakeIDP(t)
	redirect := freeLoopbackRedirect(t, "/authorization-code/callback")

	stubBrowser(t, idp, func(state string) url.Values {
		return url.Values{"code": {"good-code"}, "state": {state}}
	})

	token, err := RunOIDCLogin(context.Background(), testOIDCConfig(idp, redirect), LoginOptions{})

	require.NoError(t, err)
	assert.Equal(t, "idp-access-token", token)
	assert.Equal(t, 1, idp.tokenRequests)
}

func TestRunOIDCLogin_RejectsStateMismatch(t *testing.T) {
	idp := newFakeIDP(t)

	stubBrowser(t, idp, func(_ string) url.Values {
		return url.Values{"code": {"good-code"}, "state": {"forged"}}
	})

	_, err := RunOIDCLogin(context.Background(), testOIDCConfig(idp, freeLoopbackRedirect(t, "/")), LoginOptions{})

	require.ErrorContains(t, err, "state did not match")
	assert.Equal(t, 0, idp.tokenRequests, "no code exchange after a state mismatch")
}

func TestRunOIDCLogin_ReportsIdPError(t *testing.T) {
	idp := newFakeIDP(t)

	stubBrowser(t, idp, func(state string) url.Values {
		return url.Values{"error": {"access_denied"}, "error_description": {"User is not assigned"}, "state": {state}}
	})

	_, err := RunOIDCLogin(context.Background(), testOIDCConfig(idp, freeLoopbackRedirect(t, "/")), LoginOptions{})

	require.ErrorContains(t, err, "access_denied")
	require.ErrorContains(t, err, "User is not assigned")
}

func TestRunOIDCLogin_RejectsIssuerMismatch(t *testing.T) {
	idp := newFakeIDP(t)
	idp.issuer = "https://evil.example.com"

	_, err := RunOIDCLogin(context.Background(), testOIDCConfig(idp, freeLoopbackRedirect(t, "/")), LoginOptions{})

	require.ErrorContains(t, err, "issuer mismatch")
}

func TestDiscoverOIDC_RejectsNonDiscoveryResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing/.well-known/openid-configuration" {
			http.NotFound(w, r)

			return
		}

		_, _ = w.Write([]byte(`{"issuer": "x"}`))
	}))
	t.Cleanup(srv.Close)

	client := srv.Client()

	_, err := discoverOIDC(context.Background(), client, srv.URL+"/missing")
	require.ErrorContains(t, err, "HTTP 404")

	_, err = discoverOIDC(context.Background(), client, srv.URL+"/partial")
	require.ErrorContains(t, err, "missing authorization_endpoint")
}

func TestOIDCConfigValidate(t *testing.T) {
	valid := OIDCConfig{Issuer: "https://idp.example.com/oauth2/default", ClientID: "abc"}.WithDefaults()

	require.NoError(t, valid.Validate())
	assert.Equal(t, DefaultOIDCRedirectURI, valid.RedirectURI)
	assert.Equal(t, []string{"openid", "profile"}, valid.Scopes)

	cases := map[string]struct {
		mutate func(*OIDCConfig)
		want   string
	}{
		"missing issuer":        {func(c *OIDCConfig) { c.Issuer = "" }, "needs an issuer"},
		"plain http issuer":     {func(c *OIDCConfig) { c.Issuer = "http://idp.example.com" }, "must use https"},
		"missing client id":     {func(c *OIDCConfig) { c.ClientID = "" }, "needs a client id"},
		"non-loopback redirect": {func(c *OIDCConfig) { c.RedirectURI = "http://example.com:8080/cb" }, "loopback"},
		"https redirect":        {func(c *OIDCConfig) { c.RedirectURI = "https://localhost:8080/cb" }, "loopback"},
		"redirect without port": {func(c *OIDCConfig) { c.RedirectURI = "http://localhost/cb" }, "loopback"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)

			require.ErrorContains(t, cfg.Validate(), tc.want)
		})
	}
}

func TestOIDCConfigFromConfig(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	_, ok := OIDCConfigFromConfig()
	assert.False(t, ok, "no issuer means the API-key flow")

	StoreOIDCConfig(OIDCConfig{
		Issuer: "https://idp.example.com/", ClientID: "abc", Scopes: []string{"openid", "offline_access"},
		RedirectURI: "http://localhost:8090/authorization-code/callback",
	})

	cfg, ok := OIDCConfigFromConfig()

	require.True(t, ok)
	assert.Equal(t, "https://idp.example.com", cfg.Issuer, "trailing slash trimmed")
	assert.Equal(t, "abc", cfg.ClientID)
	assert.Equal(t, []string{"openid", "offline_access"}, cfg.Scopes)
	assert.Equal(t, "http://localhost:8090/authorization-code/callback", cfg.RedirectURI)
	assert.Equal(t, "openid offline_access", viperx.GetString(config.OAuthScopes))
}
