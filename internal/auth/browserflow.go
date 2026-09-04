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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/datarobot/cli/internal/assets"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/open"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/tui"
)

// CallbackAddr is the address the DataRobot web app redirects back to after the
// user authorizes the CLI. The port is part of the contract with the web app's
// cliRedirect handler, so it cannot be changed unilaterally and has no fallback.
const CallbackAddr = "localhost:51164"

// DefaultLoginTimeout bounds how long the CLI waits for the browser callback.
// Generous enough to cover an SSO round trip, short enough that a browser which
// never opened does not hang the terminal indefinitely.
const DefaultLoginTimeout = 5 * time.Minute

// ErrLoginInterrupted is returned when the user aborts the login (Ctrl-C) or
// another CLI process takes over the callback port.
var ErrLoginInterrupted = errors.New("login was interrupted")

// BrowserFlow owns the local HTTP listener that receives the API key after the
// user authorizes the CLI in their browser.
//
// The three steps are deliberately separate so that both the blocking CLI path
// and the bubbletea setup wizard can drive them from their own control flow:
//
//	flow, err := NewBrowserFlow(host)  // binds the listener
//	defer flow.Close()
//	openErr := flow.OpenBrowser()      // best effort; report the link if non-nil
//	key, err := flow.Wait(ctx)         // serves until the callback arrives
//
// Binding before opening the browser matters: the listener must already exist
// when the browser follows the redirect, otherwise a fast redirect races the
// CLI and the callback is refused.
type BrowserFlow struct {
	authURL  string
	listener net.Listener
	server   *http.Server
	keyCh    chan string
	timeout  time.Duration

	// OAuth mode only (nil for the legacy ?key= hand-off). When set, the
	// callback carries a code that handleCallback exchanges before publishing
	// on keyCh, so Wait's contract is identical in both modes.
	oauthMeta   *OAuthMetadata
	oauthPKCE   *pkce
	redirectURI string

	// errCh reports failures from inside the callback handler. It exists
	// because an empty string on keyCh already means "another process wants
	// this port" (see Wait), so errors cannot be signalled that way.
	errCh chan error

	// refreshToken is whatever the token exchange returned, for the caller to
	// persist. Empty in legacy mode and whenever the server issues none.
	refreshToken string

	closeOnce sync.Once
	closeErr  error
}

// NewBrowserFlow binds the callback listener and prepares the browser login for
// datarobotHost. The caller must Close the returned flow. Prefer
// NewBrowserFlowContext where a context is available.
func NewBrowserFlow(datarobotHost string) (*BrowserFlow, error) {
	return NewBrowserFlowContext(context.Background(), datarobotHost, nil)
}

// NewBrowserFlowContext binds the callback listener, choosing between the
// legacy `?key=` hand-off and PKCE.
//
// Unless someone opts in this issues no discovery request at all, which keeps
// hosts that serve a discovery document without supporting the flow working as
// before. Opted in but undiscoverable returns ErrOAuthNotSupported rather than
// falling back, which would hand over a different credential while looking
// like success.
func NewBrowserFlowContext(ctx context.Context, datarobotHost string, oauthOverride *bool) (*BrowserFlow, error) {
	if !OAuthRequested(oauthOverride) {
		return newBrowserFlowOn(CallbackAddr, datarobotHost)
	}

	meta, err := DiscoverOAuth(ctx, datarobotHost)
	if err != nil {
		return nil, err
	}

	log.Debugf("Using OAuth2 authorization-code + PKCE against %s", meta.Issuer)

	return newBrowserFlowOAuthOn(CallbackAddr, meta)
}

// newBrowserFlowOn is NewBrowserFlow with a configurable address so tests can
// bind an ephemeral port instead of competing for the fixed production one.
func newBrowserFlowOn(addr, datarobotHost string) (*BrowserFlow, error) {
	listener, err := listenReclaimingPort(addr)
	if err != nil {
		return nil, err
	}

	return newFlow(addr, listener, AuthCallbackURL(datarobotHost)), nil
}

// newFlow assembles a flow and its callback server. Both constructors go
// through here, differing only in the URL and the OAuth fields set after.
func newFlow(addr string, listener net.Listener, authURL string) *BrowserFlow {
	flow := &BrowserFlow{
		authURL:  authURL,
		listener: listener,
		keyCh:    make(chan string, 1),
		errCh:    make(chan error, 1),
		timeout:  DefaultLoginTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", flow.handleCallback)

	flow.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return flow
}

// newBrowserFlowOAuthOn binds the listener for a PKCE login. The redirect URI
// is the loopback listener itself (RFC 8252) on CallbackAddr's port, so the
// server must have http://localhost:51164/ registered for OAuthClientID.
func newBrowserFlowOAuthOn(addr string, meta *OAuthMetadata) (*BrowserFlow, error) {
	p, err := newPKCE()
	if err != nil {
		return nil, err
	}

	listener, err := listenReclaimingPort(addr)
	if err != nil {
		return nil, err
	}

	// Hostname from addr, port from the listener. listener.Addr() resolves
	// "localhost" to 127.0.0.1 and servers match redirect_uri exactly, so the
	// IP literal is rejected; the port must be the one actually bound because
	// tests bind :0.
	redirectHost, _, err := net.SplitHostPort(addr)
	if err != nil {
		listener.Close()

		return nil, fmt.Errorf("parsing callback address %q: %w", addr, err)
	}

	_, boundPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()

		return nil, fmt.Errorf("reading bound callback port: %w", err)
	}

	redirectURI := "http://" + net.JoinHostPort(redirectHost, boundPort) + "/"

	authURL, err := authorizeURL(meta, p, redirectURI)
	if err != nil { //nolint:wsl // grouped with the construction above
		listener.Close()

		return nil, err
	}

	flow := newFlow(addr, listener, authURL)
	flow.oauthMeta = meta
	flow.oauthPKCE = p
	flow.redirectURI = redirectURI

	return flow, nil
}

// AuthURL is the DataRobot URL the user must visit to authorize the CLI.
func (f *BrowserFlow) AuthURL() string {
	return f.authURL
}

// RefreshToken is from the OAuth exchange, or "" in legacy mode / when the
// server issued none. Only meaningful after Wait succeeds.
func (f *BrowserFlow) RefreshToken() string {
	return f.refreshToken
}

// TokenEndpoint is where a renewal is sent, or "" in legacy mode.
func (f *BrowserFlow) TokenEndpoint() string {
	if f.oauthMeta == nil {
		return ""
	}

	return f.oauthMeta.TokenEndpoint
}

// localAddr reports the address the listener actually bound, which differs from
// CallbackAddr only in tests.
func (f *BrowserFlow) localAddr() string {
	return f.listener.Addr().String()
}

// OpenBrowser launches the auth URL in the user's default browser.
//
// A non-nil error means the browser did not open and the caller must show the
// user the link instead; it is never fatal to the login itself, because the user
// can always follow the link by hand.
func (f *BrowserFlow) OpenBrowser() error {
	return open.Open(f.authURL)
}

// Wait serves the callback endpoint until the API key arrives, the user
// interrupts, or DefaultLoginTimeout elapses.
func (f *BrowserFlow) Wait(ctx context.Context) (string, error) {
	go func() {
		err := f.server.Serve(f.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Debugf("Auth callback server stopped: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	select {
	case apiKey := <-f.keyCh:
		// An empty key is the sentinel another CLI process uses to ask us to give
		// up the callback port.
		if apiKey == "" {
			return "", ErrLoginInterrupted
		}

		log.Debug("Successfully consumed API key from callback request")

		return apiKey, nil

	case err := <-f.errCh:
		// The callback arrived but could not be turned into a credential — a
		// state mismatch, a refused authorization, or a rejected token
		// exchange. Distinct from the empty-key sentinel above so a real
		// failure does not masquerade as an interruption.
		return "", err

	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timed out after %s waiting for browser authorization: %w", f.timeout, ctx.Err())
		}

		log.Debug("Login context cancelled, exiting auth wait")

		return "", ErrLoginInterrupted
	}
}

// Close shuts the callback server down. It is safe to call more than once.
func (f *BrowserFlow) Close() error {
	f.closeOnce.Do(func() {
		// A fresh context: the login context is typically already cancelled by the
		// time we get here (Ctrl-C), and Shutdown on a cancelled context would skip
		// draining the in-flight response that renders the success page.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := f.server.Shutdown(ctx); err != nil {
			f.closeErr = fmt.Errorf("failed to shut down auth callback server: %w", err)
		}
	})

	return f.closeErr
}

// handleCallback receives the redirect that ends the browser login: either
// `?key=<token>` (legacy) or `?code=…&state=…`, which this exchanges so Wait
// returns a usable credential either way.
//
// ORDER MATTERS. The OAuth branch is checked first because a keyless request is
// the port-reclaim interrupt sentinel (see Wait and listenReclaimingPort) — an
// OAuth callback falling through to it would abort the login.
func (f *BrowserFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	if f.oauthMeta != nil {
		if code := query.Get("code"); code != "" {
			f.handleOAuthCallback(w, r, code, query.Get("state"))

			return
		}

		// The authorization server can also report a failure on the redirect,
		// e.g. the user declining consent. Surface it rather than sitting until
		// the five-minute timeout.
		if oauthErr := query.Get("error"); oauthErr != "" {
			f.failCallback(w, fmt.Errorf("authorization was refused: %s: %s", oauthErr, query.Get("error_description")))

			return
		}

		// A `key` cannot authenticate THIS flow. Falling through to the legacy
		// handler would let any local process inject a credential during the
		// login window, bypassing the `state` check that is the whole reason
		// this flow exists.
		//
		// Refused without publishing on either channel, so the login keeps
		// waiting for the real callback: neither an injection nor a way to
		// cancel someone else's login.
		if query.Get("key") != "" {
			log.Debug("Ignoring a ?key= callback: this login is an OAuth flow")
			http.Error(w, "Unexpected credential on an OAuth callback.", http.StatusBadRequest)

			return
		}
	}

	apiKey := query.Get("key")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := assets.Write(w, "templates/success.html"); err != nil {
		log.Debugf("Failed to render auth success page: %v", err)
	}

	// Non-blocking: the channel holds one key and Wait may already have returned.
	// A stray extra callback must not park this handler goroutine forever.
	select {
	case f.keyCh <- apiKey:
	default:
		log.Debug("Discarding duplicate auth callback; a key was already received")
	}
}

// handleOAuthCallback validates the redirect and exchanges the code.
func (f *BrowserFlow) handleOAuthCallback(w http.ResponseWriter, r *http.Request, code, state string) {
	// Constant-time is unnecessary: state is single-use and anyone who can read
	// it already has the code. What matters is that a mismatch is fatal — this
	// callback is otherwise open to any local process.
	if state != f.oauthPKCE.state {
		f.failCallback(w, errors.New("OAuth state mismatch — ignoring a callback this login did not start"))

		return
	}

	tok, err := exchangeCode(r.Context(), f.oauthMeta, f.oauthPKCE, f.redirectURI, code)
	if err != nil {
		f.failCallback(w, err)

		return
	}

	// For the caller to persist after Wait.
	f.refreshToken = tok.RefreshToken

	if tok.RefreshToken == "" {
		log.Debug("Authorization server issued no refresh token; the credential expires without renewal")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if writeErr := assets.Write(w, "templates/success.html"); writeErr != nil {
		log.Debugf("Failed to render auth success page: %v", writeErr)
	}

	select {
	case f.keyCh <- tok.AccessToken:
	default:
		log.Debug("Discarding duplicate auth callback; a credential was already received")
	}
}

// failCallback tells the browser the login failed and hands the reason to Wait.
// It deliberately does not publish an empty string on keyCh: that means
// "release the port" and would turn a failure into a silent interruption.
func (f *BrowserFlow) failCallback(w http.ResponseWriter, err error) {
	log.Debugf("Auth callback failed: %v", err)

	http.Error(w, "Login failed: "+err.Error()+"\n\nReturn to the terminal for details.", http.StatusBadRequest)

	select {
	case f.errCh <- err:
	default:
		log.Debug("Discarding duplicate auth failure; one was already reported")
	}
}

// LoginOptions tunes the interactive browser login.
type LoginOptions struct {
	// NoBrowser skips launching a browser and shows the link instead. Useful over
	// SSH or anywhere the CLI cannot reach a usable browser.
	NoBrowser bool

	// OAuth forces the OAuth2 authorization-code + PKCE flow on or off. nil
	// means "not specified", deferring to DATAROBOT_OAUTH_ENABLED (default off).
	OAuth *bool
}

// RunBrowserLogin opens the browser, tells the user what is happening, and blocks
// until the DataRobot web app hands the API key back to the local listener.
//
// This is the single entry point for interactive login: both `dr auth login` and
// the implicit login triggered by EnsureAuthenticated go through it, so the two
// look the same. It binds the listener before opening the browser and only shows
// the link as a fallback, which is the whole point of CFX-6318 - the browser is
// the action being taken, not an instruction for the user to follow by hand.
func RunBrowserLogin(ctx context.Context, datarobotHost string) (string, error) {
	return RunBrowserLoginWith(ctx, datarobotHost, LoginOptions{})
}

// RunBrowserLoginWith is RunBrowserLogin with explicit options.
func RunBrowserLoginWith(ctx context.Context, datarobotHost string, opts LoginOptions) (string, error) {
	flow, err := NewBrowserFlowContext(ctx, datarobotHost, opts.OAuth)
	if err != nil {
		return "", err
	}

	// Close intentionally uses its own context rather than ctx: by the time we unwind
	// here ctx is usually already cancelled, and Shutdown on a cancelled context would
	// abandon the in-flight response that renders the success page in the browser.
	defer func() { //nolint:contextcheck // Close is deliberately detached from ctx
		if closeErr := flow.Close(); closeErr != nil {
			log.Debugf("%v", closeErr)
		}
	}()

	return runLoginWithFlow(ctx, flow, opts)
}

// runLoginWithFlow drives a flow whose listener is already bound: it opens the
// browser, shows the user the link, and waits for the callback.
//
// Split out from RunBrowserLoginWith so tests can drive a flow on an ephemeral port
// instead of competing for the fixed production one.
func runLoginWithFlow(ctx context.Context, flow *BrowserFlow, opts LoginOptions) (string, error) {
	// The browser state drives the wording: when no browser opened, the link stops
	// being a footnote and becomes the primary instruction.
	state := BrowserSkipped

	if !opts.NoBrowser {
		openErr := flow.OpenBrowser()
		if openErr != nil {
			log.Debugf("Could not open the browser automatically: %v", openErr)
		}

		state = BrowserStateFor(openErr)
	}

	// RunWithSpinner renders "<spinner> <label>", and the label may span lines, so
	// the prompt block rides along beneath the spinner without a second component.
	label := SpinnerLabel(state) + RenderBrowserPrompt(flow.AuthURL(), state)

	var apiKey string

	wait := func() error {
		var waitErr error

		apiKey, waitErr = flow.Wait(ctx)

		return waitErr
	}

	var err error

	// The link is what this command exists to produce, so it goes to stdout - where
	// the animated spinner renders its own copy (tui.Run hands bubbletea os.Stdout)
	// and where callers that redirect the streams separately look for it. The Windows
	// smoke test is one such caller; the expect-based Linux and macOS ones cannot tell
	// the difference, because a PTY merges stdout and stderr.
	//
	// Printing it here rather than leaving it to tui.RunWithSpinner, which only shows
	// the label while it animates: it drops the label outright without a terminal, and
	// diverts it to stderr under DATAROBOT_CLI_NON_INTERACTIVE. Neither leaves the
	// link on stdout, and the spinner is no use in either case anyway.
	if !reader.IsStdinTerminal() || reader.IsNonInteractive() {
		fmt.Fprintln(os.Stdout, label)

		err = wait()
	} else {
		err = tui.RunWithSpinner(label, wait)
	}

	if err != nil {
		return "", err
	}

	// Record what a later renewal needs, or clear stale material when this
	// login took the legacy path so an old refresh token is never renewed
	// against a different instance. Both call sites persist straight after.
	if flow.TokenEndpoint() != "" && flow.RefreshToken() != "" {
		StoreOAuthState(flow.RefreshToken(), flow.TokenEndpoint())
	} else {
		ClearOAuthState()
	}

	return apiKey, nil
}

// listenReclaimingPort binds addr, first asking any auth server left over from a
// previous CLI run to release it.
//
// A keyless GET to the callback endpoint is the interrupt sentinel: the stale
// process sees an empty key, aborts its own wait, and closes its listener.
func listenReclaimingPort(addr string) (net.Listener, error) {
	listener, listenErr := net.Listen("tcp", addr)
	if listenErr == nil {
		return listener, nil
	}

	log.Debugf("Auth callback port %s is busy, asking the previous process to release it", addr)

	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get("http://" + addr) //nolint:noctx // bounded by the client timeout
	if err != nil {
		log.Debugf("No auth server responded on %s: %v", addr, err)
	} else {
		_ = resp.Body.Close()
	}

	// The stale process needs a moment to unwind its wait and close the listener.
	for attempt := range 10 {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return listener, nil
		}

		if attempt < 9 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Report the original failure, which describes why the port was unavailable in
	// the first place, rather than the last retry's identical error.
	return nil, fmt.Errorf("auth callback port %s is already in use: %w", addr, listenErr)
}
