package auth

// OAuth2 authorization-code + PKCE login (RFC 8252), for deployments that front
// their own authorization server rather than the SaaS hand-off. The token
// arrives in a POST response body rather than a URL, and the callback carries a
// verifiable `state`. Opt-in, not auto-detected: some hosts serve a discovery
// document without supporting the flow, so its presence proves nothing.

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

// OAuthEnabledEnv gates the flow. Unset means no discovery request is made at
// all, so behavior is byte-identical to before.
const OAuthEnabledEnv = "DATAROBOT_OAUTH_ENABLED"

// OAuthClientID is the public client this CLI presents. No secret — there is
// nowhere safe to keep one on a user's machine — so PKCE is what protects it.
const OAuthClientID = "datarobot-cli"

const discoveryPath = "/.well-known/openid-configuration"

// `offline_access` buys renewal without a browser; a server that declines it
// simply returns no refresh token, degrading to re-login on expiry.
var oauthScopes = []string{"openid", "offline_access", "email", "profile", "groups"}

// The probe runs before anything visible happens, so it must stay short for
// hosts that do not implement discovery.
const discoveryTimeout = 5 * time.Second

const tokenExchangeTimeout = 30 * time.Second

// ErrOAuthNotSupported means no usable discovery document. Returned rather than
// falling back: the user asked for OAuth, and quietly issuing a different kind
// of credential would look like success.
var ErrOAuthNotSupported = errors.New("endpoint does not advertise an OAuth2 authorization server")

// OAuthMetadata is the subset of the discovery document this flow needs.
type OAuthMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// OAuthRequested reports whether the caller asked for the OAuth flow. override
// is the --oauth/--no-oauth flag and wins; nil defers to the environment.
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

// DiscoverOAuth fetches and validates the discovery document. Strict on
// purpose: a single-page app answers 200 with HTML for unknown paths, so
// requiring parseable JSON with both endpoints is what separates a real
// authorization server from a catch-all route.
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

// pkce is one attempt's verifier and CSRF state.
type pkce struct {
	verifier  string
	challenge string
	state     string
}

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

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

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

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// exchangeCode trades the authorization code for an access token. This is the
// step the legacy flow lacks, and why the token never appears in a URL.
func exchangeCode(ctx context.Context, meta *OAuthMetadata, p *pkce, redirectURI, code string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", p.verifier)

	tok, err := postToken(ctx, meta.TokenEndpoint, form)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	return tok, nil
}

// ErrNoRefreshToken means the profile has nothing to renew with: the legacy
// hand-off, or a server that issued none.
var ErrNoRefreshToken = errors.New("no refresh token stored for this profile")

// RefreshAccessToken renews the stored access token without a browser. Writes
// into viper but does not persist — the caller owns drconfig.yaml.
// ErrNoRefreshToken means "fall back to interactive login", not failure.
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

	// Rotating servers invalidate the old value on use, so a rotated one MUST
	// replace what is stored or the next renewal fails.
	if tok.RefreshToken != "" {
		viperx.Set(config.OAuthRefreshToken, tok.RefreshToken)
	}

	return tok.AccessToken, nil
}

// StoreOAuthState records what a later renewal needs. Persisting is the
// caller's job.
func StoreOAuthState(refreshToken, tokenEndpoint string) {
	viperx.Set(config.OAuthRefreshToken, refreshToken)
	viperx.Set(config.OAuthTokenEndpoint, tokenEndpoint)
}

// ClearOAuthState drops the renewal material. Called on logout, and whenever a
// login takes the legacy path — a refresh token left beside a hand-off
// credential would renew against whatever instance issued it, not necessarily
// the one now configured.
func ClearOAuthState() {
	viperx.Set(config.OAuthRefreshToken, "")
	viperx.Set(config.OAuthTokenEndpoint, "")
}

// postRefresh performs the refresh_token grant.
//
// A non-200 clears the stored material: a rejected refresh token is spent, so
// keeping it would make every later command retry something that cannot work.
func postRefresh(ctx context.Context, endpoint, refresh string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)

	tok, err := postToken(ctx, endpoint, form)
	if err != nil {
		// A rejected refresh token is spent, so keeping it would make every
		// later command retry something that cannot work.
		ClearOAuthState()

		return nil, fmt.Errorf("refresh rejected: %w", err)
	}

	return tok, nil
}

// postToken performs a token-endpoint grant. Shared by the authorization-code
// and refresh grants, which differ only in the form and what rejection means.
func postToken(ctx context.Context, endpoint string, form url.Values) (*tokenResponse, error) {
	form.Set("client_id", OAuthClientID)

	ctx, cancel := context.WithTimeout(ctx, tokenExchangeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling token endpoint %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tok tokenResponse

	// Decode first: error replies are JSON too, and error_description is the
	// actionable half.
	unmarshalErr := json.Unmarshal(body, &tok)

	if resp.StatusCode != http.StatusOK {
		if tok.Error != "" {
			return &tok, fmt.Errorf("%s: %s", tok.Error, tok.ErrorDescription)
		}

		return &tok, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if unmarshalErr != nil {
		return nil, fmt.Errorf("token endpoint returned unparseable JSON: %w", unmarshalErr)
	}

	if tok.AccessToken == "" {
		return nil, errors.New("token endpoint returned no access_token")
	}

	return &tok, nil
}
