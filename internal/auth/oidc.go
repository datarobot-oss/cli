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

// Direct OIDC login: authorization code + PKCE (RFC 7636) against an identity
// provider the user names (Okta, Entra, Keycloak, ...), with the redirect caught
// on a loopback listener (RFC 8252). The access token the IdP issues is stored as
// the profile's token and sent as a Bearer header like an API key, for
// deployments whose gateway validates IdP tokens directly.
//
// Unlike the API-key hand-off, nothing here talks to DataRobot: the issuer comes
// from the user, not from the DataRobot host.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/open"
)

// DefaultOIDCRedirectURI is the loopback redirect registered with the IdP. It
// reuses the API-key flow's port so one firewall or IdP entry covers both.
const DefaultOIDCRedirectURI = "http://" + CallbackAddr + "/"

// DefaultOIDCScopes is enough for an access token plus the user's identity.
// Scopes an IdP does not know (Okta custom servers reject unknown ones) are left
// for the user to add with --scopes.
var DefaultOIDCScopes = []string{"openid", "profile"}

const (
	oidcDiscoveryPath    = "/.well-known/openid-configuration"
	oidcHTTPTimeout      = 15 * time.Second
	oidcMaxResponseBytes = 1 << 20
)

// OIDCConfig is what a direct IdP login needs. Issuer and ClientID come from
// the IdP's app registration; the rest have defaults.
type OIDCConfig struct {
	Issuer      string
	ClientID    string
	Scopes      []string
	RedirectURI string
}

// WithDefaults fills the optional fields.
func (c OIDCConfig) WithDefaults() OIDCConfig {
	c.Issuer = strings.TrimRight(strings.TrimSpace(c.Issuer), "/")
	c.ClientID = strings.TrimSpace(c.ClientID)

	if len(c.Scopes) == 0 {
		c.Scopes = DefaultOIDCScopes
	}

	if strings.TrimSpace(c.RedirectURI) == "" {
		c.RedirectURI = DefaultOIDCRedirectURI
	}

	return c
}

// Validate reports a configuration the flow cannot run with, before any
// network call or browser launch.
func (c OIDCConfig) Validate() error {
	if c.Issuer == "" {
		return errors.New("an OIDC login needs an issuer: pass --issuer or set DATAROBOT_CLI_OAUTH_ISSUER")
	}

	issuer, err := url.Parse(c.Issuer)
	if err != nil || issuer.Host == "" {
		return fmt.Errorf("issuer %q is not an absolute URL", c.Issuer)
	}

	if issuer.Scheme != "https" && !isLoopbackHost(issuer.Hostname()) {
		return fmt.Errorf("issuer %q must use https", c.Issuer)
	}

	if c.ClientID == "" {
		return errors.New("an OIDC login needs a client id: pass --client-id or set DATAROBOT_CLI_OAUTH_CLIENT_ID")
	}

	_, _, err = parseLoopbackRedirect(c.RedirectURI)

	return err
}

// OIDCConfigFromConfig returns the OIDC settings saved in the active profile or
// set through DATAROBOT_CLI_OAUTH_* variables. ok is false when no issuer is
// configured, meaning the profile uses the API-key flow.
func OIDCConfigFromConfig() (OIDCConfig, bool) {
	issuer := strings.TrimSpace(viperx.GetString(config.OAuthIssuer))
	if issuer == "" {
		return OIDCConfig{}, false
	}

	cfg := OIDCConfig{
		Issuer:      issuer,
		ClientID:    viperx.GetString(config.OAuthClientID),
		Scopes:      strings.Fields(viperx.GetString(config.OAuthScopes)),
		RedirectURI: viperx.GetString(config.OAuthRedirectURI),
	}

	return cfg.WithDefaults(), true
}

// StoreOIDCConfig records the settings in viper so a later plain `dr auth
// login`, or the automatic re-login on an expired token, uses the same IdP.
// Persisting is the caller's job (see PersistOIDCConfig).
func StoreOIDCConfig(cfg OIDCConfig) {
	viperx.Set(config.OAuthIssuer, cfg.Issuer)
	viperx.Set(config.OAuthClientID, cfg.ClientID)
	viperx.Set(config.OAuthScopes, strings.Join(cfg.Scopes, " "))
	viperx.Set(config.OAuthRedirectURI, cfg.RedirectURI)
}

// PersistOIDCConfig writes the OIDC keys to drconfig.yaml, into the active
// profile's section when one is active.
func PersistOIDCConfig() error {
	return config.UpdateConfigFile(config.OAuthIssuer, config.OAuthClientID, config.OAuthScopes, config.OAuthRedirectURI)
}

// RunInteractiveLogin is the login EnsureAuthenticated falls back to: the
// direct IdP flow when the profile has an issuer, the API-key hand-off otherwise.
// Without this, an expired IdP token would send the user to the API-key page.
func RunInteractiveLogin(ctx context.Context, datarobotHost string) (string, error) {
	if cfg, ok := OIDCConfigFromConfig(); ok {
		return RunOIDCLogin(ctx, cfg, LoginOptions{})
	}

	return RunBrowserLogin(ctx, datarobotHost)
}

// RunOIDCLogin signs the user in at the IdP in their browser and returns the
// access token.
func RunOIDCLogin(ctx context.Context, cfg OIDCConfig, opts LoginOptions) (string, error) {
	cfg = cfg.WithDefaults()

	if err := cfg.Validate(); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: oidcHTTPTimeout}

	meta, err := discoverOIDC(ctx, client, cfg.Issuer)
	if err != nil {
		return "", err
	}

	flow, err := newOIDCFlow(cfg, meta)
	if err != nil {
		return "", err
	}

	defer func() { //nolint:contextcheck // Close is deliberately detached from ctx
		if closeErr := flow.Close(); closeErr != nil {
			log.Debugf("%v", closeErr)
		}
	}()

	var code string

	err = promptAndWait(flow.authURL, opts, func() error {
		var waitErr error

		code, waitErr = flow.Wait(ctx)

		return waitErr
	})
	if err != nil {
		return "", err
	}

	return exchangeCode(ctx, client, meta, cfg, code, flow.verifier)
}

// oidcMetadata is the subset of the discovery document the flow uses.
type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// discoverOIDC fetches the issuer's discovery document and checks that it
// describes that issuer (OpenID Connect Discovery 1.0 section 4.3), so a wrong
// or hijacked document cannot redirect the login to another server.
func discoverOIDC(ctx context.Context, client *http.Client, issuer string) (*oidcMetadata, error) {
	discoveryURL := issuer + oidcDiscoveryPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building discovery request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", discoveryURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d (is the issuer right?)", discoveryURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, oidcMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", discoveryURL, err)
	}

	var meta oidcMetadata

	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("%s is not a discovery document: %w", discoveryURL, err)
	}

	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("%s is missing authorization_endpoint or token_endpoint", discoveryURL)
	}

	if strings.TrimRight(meta.Issuer, "/") != issuer {
		return nil, fmt.Errorf("issuer mismatch: %s describes %q, not %q", discoveryURL, meta.Issuer, issuer)
	}

	return &meta, nil
}

type oidcResult struct {
	code string
	err  error
}

// oidcFlow owns the loopback listener that receives the authorization code.
type oidcFlow struct {
	authURL      string
	verifier     string
	state        string
	redirectPath string
	listener     net.Listener
	server       *http.Server
	resultCh     chan oidcResult
	timeout      time.Duration

	closeOnce sync.Once
	closeErr  error
}

func newOIDCFlow(cfg OIDCConfig, meta *oidcMetadata) (*oidcFlow, error) {
	addr, redirectPath, err := parseLoopbackRedirect(cfg.RedirectURI)
	if err != nil {
		return nil, err
	}

	verifier, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}

	state, err := randomURLSafe(16)
	if err != nil {
		return nil, err
	}

	authURL, err := buildAuthorizeURL(meta.AuthorizationEndpoint, cfg, state, pkceChallenge(verifier))
	if err != nil {
		return nil, err
	}

	listener, err := listenReclaimingPort(addr)
	if err != nil {
		return nil, err
	}

	flow := &oidcFlow{
		authURL:      authURL,
		verifier:     verifier,
		state:        state,
		redirectPath: redirectPath,
		listener:     listener,
		resultCh:     make(chan oidcResult, 1),
		timeout:      DefaultLoginTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", flow.handleCallback)

	flow.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return flow, nil
}

// Wait serves the callback until the code arrives, the user interrupts, or the
// login times out.
func (f *oidcFlow) Wait(ctx context.Context) (string, error) {
	go func() {
		err := f.server.Serve(f.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Debugf("OIDC callback server stopped: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	select {
	case res := <-f.resultCh:
		return res.code, res.err

	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timed out after %s waiting for the identity provider: %w", f.timeout, ctx.Err())
		}

		return "", ErrLoginInterrupted
	}
}

// Close shuts the callback server down. It is safe to call more than once.
func (f *oidcFlow) Close() error {
	f.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := f.server.Shutdown(ctx); err != nil {
			f.closeErr = fmt.Errorf("failed to shut down OIDC callback server: %w", err)
		}
	})

	return f.closeErr
}

const oidcSuccessPage = `<!doctype html><html><head><meta charset="utf-8"><title>Signed in</title></head>` +
	`<body style="font-family:-apple-system,system-ui,sans-serif;text-align:center;padding:4rem">` +
	`<h1>Signed in</h1><p>The DataRobot CLI has your token. You can close this tab.</p></body></html>`

// handleCallback receives the IdP's redirect. A request carrying neither a code
// nor an error is the port-handover sentinel listenReclaimingPort sends (shared
// with the API-key flow), so a newer login can take the port over.
func (f *oidcFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "document" {
		http.Error(w, "forbidden: request type not allowed", http.StatusForbidden)

		return
	}

	res, deliver := f.respond(w, r)
	if !deliver {
		return
	}

	select {
	case f.resultCh <- res:
	default:
		log.Debug("Discarding duplicate OIDC callback")
	}
}

// respond answers the browser and returns what Wait should see. deliver is
// false for requests that are neither the callback nor the handover sentinel.
func (f *oidcFlow) respond(w http.ResponseWriter, r *http.Request) (oidcResult, bool) {
	q := r.URL.Query()
	code, idpErr := q.Get("code"), q.Get("error")

	switch {
	case code == "" && idpErr == "":
		w.WriteHeader(http.StatusNoContent)

		return oidcResult{err: ErrLoginInterrupted}, true

	case r.URL.Path != f.redirectPath:
		http.NotFound(w, r)

		return oidcResult{}, false

	case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(f.state)) != 1:
		http.Error(w, "login failed: state mismatch; start the login again", http.StatusBadRequest)

		return oidcResult{err: errors.New("callback state did not match; the login was not completed")}, true

	case idpErr != "":
		http.Error(w, "login failed: "+idpErr, http.StatusBadRequest)

		return oidcResult{err: fmt.Errorf("identity provider returned %s: %s", idpErr, q.Get("error_description"))}, true

	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, oidcSuccessPage)

		return oidcResult{code: code}, true
	}
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// exchangeCode redeems the authorization code at the token endpoint. Public
// client: no secret, the PKCE verifier is the proof.
func exchangeCode(ctx context.Context, client *http.Client, meta *oidcMetadata, cfg OIDCConfig, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling the token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, oidcMaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading the token response: %w", err)
	}

	var tok tokenResponse

	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("token endpoint returned HTTP %d and no JSON", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK || tok.Error != "" {
		return "", fmt.Errorf("token endpoint returned HTTP %d: %s %s", resp.StatusCode, tok.Error, tok.ErrorDescription)
	}

	if tok.AccessToken == "" {
		return "", errors.New("token endpoint returned no access_token")
	}

	log.Debugf("OIDC login succeeded; access token expires in %ds", tok.ExpiresIn)

	return tok.AccessToken, nil
}

func buildAuthorizeURL(endpoint string, cfg OIDCConfig, state, challenge string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing authorization_endpoint %q: %w", endpoint, err)
	}

	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("scope", strings.Join(cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// parseLoopbackRedirect returns the listen address and path for an http
// loopback redirect URI. Anything else would hand the code to another machine.
func parseLoopbackRedirect(redirectURI string) (string, string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", "", fmt.Errorf("redirect URI %q: %w", redirectURI, err)
	}

	if u.Scheme != "http" || !isLoopbackHost(u.Hostname()) || u.Port() == "" {
		return "", "", fmt.Errorf("redirect URI %q must be http://localhost:<port>/... (a loopback address with a port)", redirectURI)
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	return net.JoinHostPort(u.Hostname(), u.Port()), path, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// openBrowser is a seam so tests do not launch a real browser.
var openBrowser = open.Open
