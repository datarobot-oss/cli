package auth

// OAuth2 authorization-code + PKCE login, for DataRobot deployments that front
// their own OAuth2 authorization server rather than the SaaS
// `/account/developer-tools?cliRedirect=true` hand-off.
//
// Why this exists alongside the legacy flow rather than replacing it: the SaaS
// path returns the credential as a `?key=<token>` query parameter, which means
// the token itself travels in a URL and lands in browser history. Here the CLI
// holds a PKCE verifier, receives only an authorization code on the loopback
// redirect, and exchanges it for the token over POST — so the token arrives in a
// response body. It also gives the callback a real `state` to check, which the
// legacy path has no way to provide.
//
// The legacy path remains the DEFAULT. This one activates only when explicitly
// asked for, because some deployments serve an OIDC discovery document without
// having a login service behind it: the document's presence is not evidence the
// flow is supported, so it cannot be used as an auto-detect signal.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

// OAuthEnabledEnv gates the discovery lookup. Unset (the default) means the CLI
// behaves exactly as it always has and issues no discovery request at all.
const OAuthEnabledEnv = "DATAROBOT_OAUTH_ENABLED"

// OAuthClientID is the public OAuth2 client this CLI presents. Public means no
// client secret — there is nowhere safe to keep one on a user's machine — so the
// authorization server is expected to require PKCE for it.
const OAuthClientID = "datarobot-cli"

// discoveryPath is the RFC 8414 / OpenID Connect Discovery location, resolved
// against the configured endpoint's scheme+host.
const discoveryPath = "/.well-known/openid-configuration"

// oauthScopes are requested on every authorization.
//
// `offline_access` is requested deliberately: deployments may issue short-lived
// access tokens, and without a refresh token the user would have to re-run
// `dr auth login` every time one expired. An authorization server that does not
// grant it simply returns no refresh token, which degrades to the same
// re-login-on-expiry behavior the legacy flow already has.
var oauthScopes = []string{"openid", "offline_access", "email", "profile", "groups"}

// discoveryTimeout bounds the probe. It runs before anything visible happens, so
// it must not be something a user waits on when the endpoint does not implement
// discovery at all.
const discoveryTimeout = 5 * time.Second

// tokenExchangeTimeout bounds the code-for-token POST. This one runs inside the
// callback HTTP handler, with the user watching a browser tab, so it is short.
const tokenExchangeTimeout = 30 * time.Second

// ErrOAuthNotSupported means the endpoint did not serve a usable discovery
// document. It is returned rather than silently falling back to the legacy flow:
// the user explicitly asked for OAuth, and quietly handing them a different kind
// of credential instead would look like success.
var ErrOAuthNotSupported = errors.New("endpoint does not advertise an OAuth2 authorization server")

// OAuthMetadata is the subset of the discovery document this flow needs.
type OAuthMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// OAuthRequested reports whether the caller asked for the OAuth flow.
//
// override comes from an explicit --oauth/--no-oauth flag and wins outright;
// nil means "not specified", in which case the environment decides. Default off.
func OAuthRequested(override *bool) bool {
	if override != nil {
		return *override
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv(OAuthEnabledEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// DiscoverOAuth fetches and validates the discovery document for datarobotHost.
//
// It is strict on purpose. A single-page app that serves its shell for unknown
// paths answers 200 with HTML for this URL, which would otherwise look like a
// successful discovery; requiring parseable JSON carrying both endpoints is what
// separates a real authorization server from a catch-all route.
func DiscoverOAuth(ctx context.Context, datarobotHost string) (*OAuthMetadata, error) {
	base := strings.TrimRight(datarobotHost, "/")

	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+discoveryPath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOAuthNotSupported, err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrOAuthNotSupported, base+discoveryPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s returned HTTP %d", ErrOAuthNotSupported, base+discoveryPath, resp.StatusCode)
	}

	// Bounded read: this is an untrusted endpoint and the document is small.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %w", ErrOAuthNotSupported, base+discoveryPath, err)
	}

	var meta OAuthMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("%w: %s did not return JSON (an SPA catch-all route?)", ErrOAuthNotSupported, base+discoveryPath)
	}

	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("%w: %s is missing authorization_endpoint or token_endpoint", ErrOAuthNotSupported, base+discoveryPath)
	}

	log.Debugf("OAuth discovery at %s: issuer=%s", base+discoveryPath, meta.Issuer)

	return &meta, nil
}

// pkce carries one authorization attempt's PKCE verifier and CSRF state.
type pkce struct {
	verifier  string
	challenge string
	state     string
}

// newPKCE generates a fresh verifier/challenge/state triple.
func newPKCE() (*pkce, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return nil, fmt.Errorf("generating PKCE verifier: %w", err)
	}

	state, err := randomURLSafe(16)
	if err != nil {
		return nil, fmt.Errorf("generating OAuth state: %w", err)
	}

	sum := sha256.Sum256([]byte(verifier))

	return &pkce{
		verifier:  verifier,
		challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
		state:     state,
	}, nil
}

// randomURLSafe returns n bytes of crypto-random data, base64url encoded.
func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// authorizeURL builds the browser-facing authorization request.
func authorizeURL(meta *OAuthMetadata, p *pkce, redirectURI string) (string, error) {
	u, err := url.Parse(meta.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("parsing authorization_endpoint %q: %w", meta.AuthorizationEndpoint, err)
	}

	q := u.Query()
	q.Set("client_id", OAuthClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(oauthScopes, " "))
	q.Set("state", p.state)
	q.Set("code_challenge", p.challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// tokenResponse is the subset of the token endpoint's reply we consume.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// exchangeCode trades the authorization code for an access token.
//
// This is the step the legacy flow does not have, and the reason the token never
// appears in a URL: it comes back in this POST's response body.
func exchangeCode(ctx context.Context, meta *OAuthMetadata, p *pkce, redirectURI, code string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", OAuthClientID)
	form.Set("code_verifier", p.verifier)

	ctx, cancel := context.WithTimeout(ctx, tokenExchangeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code at %s: %w", meta.TokenEndpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tok tokenResponse
	// Decode before checking the status: OAuth2 error replies are JSON too, and
	// their `error_description` is far more useful than a bare status code.
	if err := json.Unmarshal(body, &tok); err != nil && resp.StatusCode == http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned unparseable JSON: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if tok.Error != "" {
			return nil, fmt.Errorf("token exchange rejected: %s: %s", tok.Error, tok.ErrorDescription)
		}

		return nil, fmt.Errorf("token exchange failed with HTTP %d", resp.StatusCode)
	}

	if tok.AccessToken == "" {
		return nil, errors.New("token endpoint returned no access_token")
	}

	return &tok, nil
}

// ErrNoRefreshToken means this profile has nothing to renew with — either it
// was authenticated by the legacy hand-off, or the authorization server did not
// issue a refresh token.
var ErrNoRefreshToken = errors.New("no refresh token stored for this profile")

// RefreshAccessToken renews the stored access token without a browser.
//
// This is the whole point of asking for `offline_access`: an authorization
// server that issues short-lived access tokens would otherwise force a full
// interactive login every time one expired.
//
// It writes the new credential into viper but does NOT persist it — the caller
// owns writing drconfig.yaml, matching how the login flow behaves. Returns
// ErrNoRefreshToken when the profile has nothing to renew with, which callers
// should treat as "fall back to interactive login", not as a failure.
func RefreshAccessToken(ctx context.Context) (string, error) {
	refresh := viperx.GetString(config.OAuthRefreshToken)
	endpoint := viperx.GetString(config.OAuthTokenEndpoint)

	if refresh == "" || endpoint == "" {
		return "", ErrNoRefreshToken
	}

	tok, err := postRefresh(ctx, endpoint, refresh)
	if err != nil {
		return "", err
	}

	viperx.Set(config.DataRobotAPIKey, tok.AccessToken)

	// Servers that rotate refresh tokens invalidate the old one on use, so a
	// rotated value MUST replace what is stored or the next renewal fails.
	// Servers that do not rotate simply omit it, and the stored one stays good.
	if tok.RefreshToken != "" {
		viperx.Set(config.OAuthRefreshToken, tok.RefreshToken)
	}

	return tok.AccessToken, nil
}

// StoreOAuthState records what a successful OAuth login needs for later
// renewal. Persisting is the caller's job.
func StoreOAuthState(refreshToken, tokenEndpoint string) {
	viperx.Set(config.OAuthRefreshToken, refreshToken)
	viperx.Set(config.OAuthTokenEndpoint, tokenEndpoint)
}

// ClearOAuthState drops the renewal material.
//
// Called on logout, and whenever a login takes the legacy path: a refresh token
// left beside a hand-off credential would later be renewed against whatever
// instance issued it, which is not necessarily the one now configured.
func ClearOAuthState() {
	viperx.Set(config.OAuthRefreshToken, "")
	viperx.Set(config.OAuthTokenEndpoint, "")
}

// postRefresh performs the refresh_token grant and returns the parsed response.
//
// A non-200 clears the stored material before returning: a rejected refresh
// token is spent — revoked, expired, or already rotated — so keeping it would
// make every later command retry something that cannot work.
func postRefresh(ctx context.Context, endpoint, refresh string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", OAuthClientID)

	ctx, cancel := context.WithTimeout(ctx, tokenExchangeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing access token at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	var tok tokenResponse

	// Decode before checking the status: OAuth2 error replies are JSON too, and
	// their error_description is the actionable half.
	unmarshalErr := json.Unmarshal(body, &tok)

	if resp.StatusCode != http.StatusOK {
		ClearOAuthState()

		if tok.Error != "" {
			return nil, fmt.Errorf("refresh rejected: %s: %s", tok.Error, tok.ErrorDescription)
		}

		return nil, fmt.Errorf("refresh failed with HTTP %d", resp.StatusCode)
	}

	if unmarshalErr != nil {
		return nil, fmt.Errorf("refresh endpoint returned unparseable JSON: %w", unmarshalErr)
	}

	if tok.AccessToken == "" {
		return nil, errors.New("refresh endpoint returned no access_token")
	}

	return &tok, nil
}
