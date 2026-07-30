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

	closeOnce sync.Once
	closeErr  error
}

// NewBrowserFlow binds the callback listener and prepares the browser login for
// datarobotHost. The caller must Close the returned flow.
func NewBrowserFlow(datarobotHost string) (*BrowserFlow, error) {
	return newBrowserFlowOn(CallbackAddr, datarobotHost)
}

// newBrowserFlowOn is NewBrowserFlow with a configurable address so tests can
// bind an ephemeral port instead of competing for the fixed production one.
func newBrowserFlowOn(addr, datarobotHost string) (*BrowserFlow, error) {
	listener, err := listenReclaimingPort(addr)
	if err != nil {
		return nil, err
	}

	flow := &BrowserFlow{
		authURL:  AuthCallbackURL(datarobotHost),
		listener: listener,
		keyCh:    make(chan string, 1),
		timeout:  DefaultLoginTimeout,
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

// AuthURL is the DataRobot URL the user must visit to authorize the CLI.
func (f *BrowserFlow) AuthURL() string {
	return f.authURL
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

// handleCallback receives the redirect from the DataRobot web app, which carries
// the API key as the "key" query parameter.
func (f *BrowserFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("key")

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

// LoginOptions tunes the interactive browser login.
type LoginOptions struct {
	// NoBrowser skips launching a browser and shows the link instead. Useful over
	// SSH or anywhere the CLI cannot reach a usable browser.
	NoBrowser bool
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
	flow, err := NewBrowserFlow(datarobotHost)
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

	// tui.RunWithSpinner drops the label entirely when stdin is not a terminal, which
	// would leave a piped or redirected invocation waiting in silence with no link to
	// follow. Print it ourselves in that case; the animated spinner is only useful on
	// a real terminal anyway.
	if !reader.IsStdinTerminal() {
		fmt.Fprintln(os.Stderr, label)

		err = wait()
	} else {
		err = tui.RunWithSpinner(label, wait)
	}

	if err != nil {
		return "", err
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
