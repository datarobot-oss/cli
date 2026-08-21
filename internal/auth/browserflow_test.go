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
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestFlow binds a flow on an ephemeral port so tests never collide with the
// fixed production port (or with each other when run in parallel).
func newTestFlow(t *testing.T) *BrowserFlow {
	t.Helper()

	flow, err := newBrowserFlowOn("127.0.0.1:0", "https://app.datarobot.com")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = flow.Close()
	})

	return flow
}

// callbackURL builds the URL the browser would be redirected to, using the port
// the test flow actually bound.
func callbackURL(t *testing.T, flow *BrowserFlow, query string) string {
	t.Helper()

	return "http://" + flow.localAddr() + "/" + query
}

func TestBrowserFlow_AuthURL(t *testing.T) {
	flow := newTestFlow(t)

	assert.Equal(t,
		"https://app.datarobot.com/account/developer-tools?cliRedirect=true",
		flow.AuthURL(),
		"AuthURL must match AuthCallbackURL so the CLI and the wizard show the same link",
	)
}

func TestBrowserFlow_WaitReturnsKeyFromCallback(t *testing.T) {
	flow := newTestFlow(t)

	keyCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		key, err := flow.Wait(context.Background())

		keyCh <- key

		errCh <- err
	}()

	resp := waitForCallback(t, callbackURL(t, flow, "?key=test-api-key"))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html",
		"the browser must receive the rendered success page")
	assert.NotEmpty(t, body, "success.html should have been written to the response")

	require.NoError(t, <-errCh)
	assert.Equal(t, "test-api-key", <-keyCh)
}

func TestBrowserFlow_EmptyKeyIsInterrupt(t *testing.T) {
	// A bare GET with no key is how a newly started CLI tells a stale auth server
	// from a previous run to give up the port.
	flow := newTestFlow(t)

	errCh := make(chan error, 1)

	go func() {
		_, err := flow.Wait(context.Background())
		errCh <- err
	}()

	resp := waitForCallback(t, callbackURL(t, flow, ""))
	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, body, "the sentinel response must not be the success page")

	err := <-errCh
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoginInterrupted)
}

func TestBrowserFlow_RefusesForgedBrowserCallbacks(t *testing.T) {
	// An <img> carries Sec-Fetch-Dest "image" and a fetch()/XHR carries "empty";
	// a page cannot forge these headers, so neither can plant a key (CFX-7754).
	for _, dest := range []string{"image", "empty"} {
		t.Run(dest, func(t *testing.T) {
			flow := newTestFlow(t)

			keyCh := make(chan string, 1)
			errCh := make(chan error, 1)

			go func() {
				key, err := flow.Wait(context.Background())

				keyCh <- key

				errCh <- err
			}()

			forged := waitForCallback(t, callbackURL(t, flow, "?key=planted-by-attacker"),
				"Sec-Fetch-Dest", dest)
			require.NoError(t, forged.Body.Close())
			assert.Equal(t, http.StatusForbidden, forged.StatusCode)

			// The login must still be waiting: a genuine headerless callback completes
			// it, and the planted key never surfaces.
			genuine := waitForCallback(t, callbackURL(t, flow, "?key=real-key"))
			require.NoError(t, genuine.Body.Close())

			require.NoError(t, <-errCh)
			assert.Equal(t, "real-key", <-keyCh)
		})
	}
}

func TestBrowserFlow_AcceptsDocumentNavigation(t *testing.T) {
	// The real callback is a top-level navigation, marked Sec-Fetch-Dest "document".
	flow := newTestFlow(t)

	keyCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		key, err := flow.Wait(context.Background())

		keyCh <- key

		errCh <- err
	}()

	resp := waitForCallback(t, callbackURL(t, flow, "?key=real-key"), "Sec-Fetch-Dest", "document")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, body, "the navigation must still receive the success page")

	require.NoError(t, <-errCh)
	assert.Equal(t, "real-key", <-keyCh)
}

func TestBrowserFlow_KeylessBrowserProbeDoesNotInterrupt(t *testing.T) {
	// A favicon probe is keyless with Sec-Fetch-Dest "image". It used to be mistaken
	// for the port-handover sentinel and aborted the login in progress.
	flow := newTestFlow(t)

	keyCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		key, err := flow.Wait(context.Background())

		keyCh <- key

		errCh <- err
	}()

	probe := waitForCallback(t, callbackURL(t, flow, ""), "Sec-Fetch-Dest", "image")
	require.NoError(t, probe.Body.Close())
	assert.Equal(t, http.StatusForbidden, probe.StatusCode)

	resp := waitForCallback(t, callbackURL(t, flow, "?key=real-key"))
	require.NoError(t, resp.Body.Close())

	require.NoError(t, <-errCh)
	assert.Equal(t, "real-key", <-keyCh)
}

func TestBrowserFlow_WaitHonoursContextCancellation(t *testing.T) {
	flow := newTestFlow(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)

	go func() {
		_, err := flow.Wait(ctx)
		errCh <- err
	}()

	cancel()

	err := <-errCh
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoginInterrupted, "Ctrl-C should surface as an interrupt, not a timeout")
}

func TestBrowserFlow_WaitTimesOut(t *testing.T) {
	// Before this change the wait was unbounded: a browser that never opened left
	// the CLI blocked forever with no explanation.
	flow := newTestFlow(t)
	flow.timeout = 50 * time.Millisecond

	_, err := flow.Wait(context.Background())

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, ErrLoginInterrupted,
		"a timeout must be distinguishable from a user interrupt")
}

func TestBrowserFlow_ExtraCallbacksDoNotBlockHandlers(t *testing.T) {
	// The key channel is buffered for exactly one key. Once Wait has returned and
	// nobody is draining it, further callbacks must still be answered and dropped
	// rather than parking HTTP handler goroutines forever.
	flow := newTestFlow(t)

	errCh := make(chan error, 1)

	go func() {
		_, err := flow.Wait(context.Background())
		errCh <- err
	}()

	first := waitForCallback(t, callbackURL(t, flow, "?key=first"))
	require.NoError(t, first.Body.Close())
	require.NoError(t, <-errCh)

	// The first extra fills the now-empty buffer; the second has nowhere to go and
	// is the one that would deadlock a naive blocking send.
	for _, key := range []string{"extra-one", "extra-two", "extra-three"} {
		done := make(chan struct{})

		go func() {
			defer close(done)

			resp, err := http.Get(callbackURL(t, flow, "?key="+key)) //nolint:noctx // short-lived test request
			if err == nil {
				_ = resp.Body.Close()
			}
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("callback %q blocked: the handler is leaking on a full channel", key)
		}
	}
}

func TestBrowserFlow_CloseIsIdempotent(t *testing.T) {
	flow, err := newBrowserFlowOn("127.0.0.1:0", "https://app.datarobot.com")
	require.NoError(t, err)

	require.NoError(t, flow.Close())
	assert.NoError(t, flow.Close(), "Close must be safe to call twice (defer plus explicit)")
}

func TestBrowserFlow_ReclaimsPortFromStaleServer(t *testing.T) {
	// Two CLI runs must not deadlock on the fixed callback port: starting a second
	// flow on the same address interrupts the first and takes over.
	first, err := newBrowserFlowOn("127.0.0.1:0", "https://app.datarobot.com")
	require.NoError(t, err)

	addr := first.localAddr()

	firstErrCh := make(chan error, 1)

	// Mirror what real callers do: Close once Wait returns. Releasing the listener
	// is what actually frees the port for the incoming process.
	go func() {
		defer func() {
			_ = first.Close()
		}()

		_, waitErr := first.Wait(context.Background())
		firstErrCh <- waitErr
	}()

	// No HTTP readiness probe here: any request to the callback server counts as a
	// keyless callback and would itself interrupt the first flow. The port is held
	// from construction, so the second flow's takeover retry absorbs the timing.
	second, err := newBrowserFlowOn(addr, "https://app.datarobot.com")
	require.NoError(t, err, "a second flow must be able to reclaim the port")

	t.Cleanup(func() {
		_ = second.Close()
	})

	select {
	case waitErr := <-firstErrCh:
		require.ErrorIs(t, waitErr, ErrLoginInterrupted, "the stale flow should report an interrupt")
	case <-time.After(5 * time.Second):
		t.Fatal("the stale flow was never interrupted")
	}
}

// waitForCallback issues the callback request, retrying briefly because Wait
// starts the server asynchronously. headers are alternating name/value pairs.
func waitForCallback(t *testing.T, url string, headers ...string) *http.Response {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		require.NoError(t, reqErr)

		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp
		}

		if time.Now().After(deadline) {
			require.NoError(t, err, "callback server never became reachable at %s", url)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestBrowserFlow_OpenBrowserIsNoOpUnderTest(t *testing.T) {
	flow := newTestFlow(t)

	// open.Open short-circuits under test, so this asserts the wiring compiles and
	// reports success rather than that a browser actually launched.
	assert.NoError(t, flow.OpenBrowser())
}

func TestErrLoginInterrupted_IsNotDeadlineExceeded(t *testing.T) {
	assert.NotErrorIs(t, ErrLoginInterrupted, context.DeadlineExceeded)
}

// captureStdoutStderr runs fn with both standard streams replaced by pipes and
// returns what each received.
//
// The streams are swapped rather than injected because the login prompt is written
// through os.Stdout/os.Stderr directly, which is the behaviour under test: the
// Windows smoke test reads the two streams as separate files, so which one the link
// lands on is a contract, not an implementation detail.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	require.NoError(t, err)

	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
	}()

	// Drain concurrently: a prompt larger than the pipe buffer would otherwise block
	// fn forever.
	outCh, errCh := make(chan string, 1), make(chan string, 1)

	go func() { outCh <- readAll(outR) }()
	go func() { errCh <- readAll(errR) }()

	fn()

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())

	return <-outCh, <-errCh
}

func readAll(r io.Reader) string {
	b, _ := io.ReadAll(r)

	return string(b)
}

// TestRunLoginWithFlow_PrintsLinkToStdout pins which stream carries the login link
// when there is no terminal to animate a spinner on.
//
// It has to be stdout. The Windows smoke test launches `dr auth login` with
// Start-Process, redirects the two streams to separate files, and polls only the
// stdout one for "cliRedirect=true" - so a link on stderr fails the test while
// looking correct to anyone watching a terminal. The expect-based Linux and macOS
// tests cannot catch this: a PTY merges both streams into one.
func TestRunLoginWithFlow_PrintsLinkToStdout(t *testing.T) {
	flow := newTestFlow(t)

	// Already cancelled, so Wait returns at once instead of holding the test for the
	// full login timeout. The prompt is printed before the wait either way.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, stderr := captureStdoutStderr(t, func() {
		_, err := runLoginWithFlow(ctx, flow, LoginOptions{NoBrowser: true})
		assert.ErrorIs(t, err, ErrLoginInterrupted)
	})

	assert.Contains(t, stdout, "cliRedirect=true",
		"the smoke test polls stdout for this substring")
	assert.Contains(t, stdout, flow.AuthURL(),
		"the whole link must reach stdout unbroken")
	assert.NotContains(t, stderr, "cliRedirect=true",
		"the link must not be diverted to stderr, where redirected callers miss it")
}
