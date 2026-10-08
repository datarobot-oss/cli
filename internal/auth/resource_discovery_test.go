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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveResource answers /.well-known/oauth-protected-resource with whatever
// body builds from the server's own URL.
func serveResource(t *testing.T, status int, contentType string, body func(self string) string) *httptest.Server {
	t.Helper()

	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protectedResourcePath {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body(srv.URL)))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestDiscoverOIDCFromResource_UsesAdvertisedIdP(t *testing.T) {
	srv := serveResource(t, http.StatusOK, "application/json", func(self string) string {
		return `{"resource":"` + self + `","authorization_servers":["https://idp.example.com/oauth2/aus1"],` +
			`"datarobot_cli":{"client_id":"0oa1","redirect_uri":"http://localhost:8090/cb","scopes":["openid","profile"]}}`
	})

	cfg, ok := DiscoverOIDCFromResource(context.Background(), srv.URL)

	require.True(t, ok)
	assert.Equal(t, "https://idp.example.com/oauth2/aus1", cfg.Issuer)
	assert.Equal(t, "0oa1", cfg.ClientID)
	assert.Equal(t, "http://localhost:8090/cb", cfg.RedirectURI)
	assert.Equal(t, []string{"openid", "profile"}, cfg.Scopes)
}

func TestDiscoverOIDCFromResource_DefaultsMissingOptionalFields(t *testing.T) {
	srv := serveResource(t, http.StatusOK, "application/json", func(self string) string {
		return `{"resource":"` + self + `/","authorization_servers":["https://idp.example.com"],"datarobot_cli":{"client_id":"0oa1"}}`
	})

	cfg, ok := DiscoverOIDCFromResource(context.Background(), srv.URL)

	require.True(t, ok)
	assert.Equal(t, DefaultOIDCRedirectURI, cfg.RedirectURI)
	assert.Equal(t, DefaultOIDCScopes, cfg.Scopes)
}

// Every case here must fall back to the API-key flow (ok == false).
func TestDiscoverOIDCFromResource_FallsBack(t *testing.T) {
	cases := map[string]struct {
		status int
		ctype  string
		body   func(self string) string
	}{
		"resource names another host": {http.StatusOK, "application/json", func(string) string {
			return `{"resource":"https://other.example.com","authorization_servers":["https://idp.example.com"],"datarobot_cli":{"client_id":"0oa1"}}`
		}},
		"plain RFC 9728 document for MCP clients, no datarobot_cli": {http.StatusOK, "application/json", func(self string) string {
			return `{"resource":"` + self + `","authorization_servers":["https://idp.example.com"]}`
		}},
		"no authorization server": {http.StatusOK, "application/json", func(self string) string {
			return `{"resource":"` + self + `","datarobot_cli":{"client_id":"0oa1"}}`
		}},
		"advertised issuer is plain http": {http.StatusOK, "application/json", func(self string) string {
			return `{"resource":"` + self + `","authorization_servers":["http://idp.example.com"],"datarobot_cli":{"client_id":"0oa1"}}`
		}},
		"SPA catch-all HTML": {http.StatusOK, "text/html", func(string) string { return "<!doctype html><html></html>" }},
		"not served":         {http.StatusNotFound, "text/plain", func(string) string { return "not found" }},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := serveResource(t, tc.status, tc.ctype, tc.body)

			_, ok := DiscoverOIDCFromResource(context.Background(), srv.URL)

			assert.False(t, ok)
		})
	}
}

func TestDiscoverOIDCFromResource_DoesNotFollowRedirects(t *testing.T) {
	target := serveResource(t, http.StatusOK, "application/json", func(self string) string {
		return `{"resource":"` + self + `","authorization_servers":["https://idp.example.com"],"datarobot_cli":{"client_id":"0oa1"}}`
	})

	redirector := httptest.NewServer(http.RedirectHandler(target.URL+protectedResourcePath, http.StatusFound))
	t.Cleanup(redirector.Close)

	_, ok := DiscoverOIDCFromResource(context.Background(), redirector.URL)

	assert.False(t, ok)
}

func TestDiscoverOIDCFromResource_RefusesPlainHTTPRemoteHost(t *testing.T) {
	_, ok := DiscoverOIDCFromResource(context.Background(), "http://datarobot.example.com")

	assert.False(t, ok)
}
