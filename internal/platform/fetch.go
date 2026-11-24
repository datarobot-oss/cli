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

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

const (
	// callTimeout bounds one request, so a source that hangs does not hold the
	// report back.
	callTimeout = 15 * time.Second

	// maxBodyBytes is the most one response may hold. The largest recorded
	// answer is a few hundred kilobytes.
	maxBodyBytes = 8 << 20

	// maxPages and maxItems bound one paged read. A page holds up to listLimit
	// items, so maxItems is also the most a full run of pages can hold.
	maxPages = 100
	maxItems = 10_000

	// errorBodyBytes is how much of an error response the report keeps. The
	// read goes further than the cut, so redaction sees a token the cut would
	// have split.
	errorBodyBytes = 512
	errorReadBytes = 2 * errorBodyBytes

	// maxRedirects is the number of redirects one request follows, as net/http
	// does by default.
	maxRedirects = 10

	// maxRetries is how many times a throttled request is sent again. Every
	// request this command sends reads state, so each is safe to repeat.
	maxRetries = 2

	// retryBase and retryCap shape the wait when the server names none: full
	// jitter, a random wait up to retryBase doubled per attempt, never above
	// retryCap. retryAfterCap is the longest wait the server may ask for. A
	// longer one is reported instead of waited for, so a run stays short.
	retryBase     = 250 * time.Millisecond
	retryCap      = 2 * time.Second
	retryAfterCap = 5 * time.Second
)

// sleep waits for d or until ctx ends, and is a seam for tests.
var sleep = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// jitter returns a random duration in [0, upper], a seam for tests.
var jitter = func(upper time.Duration) time.Duration {
	return rand.N(upper + 1) //nolint:gosec // a retry delay needs no cryptographic randomness
}

// apiURL builds a URL under the configured install's /api/v2.
func apiURL(path string, query url.Values) (string, error) {
	return drapi.EndpointURL(path, query)
}

// publicURL builds a URL at the configured install's root, where the
// unauthenticated /config route lives.
func publicURL(path string) (string, error) {
	return config.GetEndpointURL(path)
}

// transportError is a failure to get a usable answer: the install is
// unreachable, the name does not resolve, TLS failed, a redirect left the
// install, or the answer was not the JSON the route promises. A failure that
// never left the machine, such as an empty endpoint or a token that cannot be
// resolved, is not one.
type transportError struct{ cause error }

func (e *transportError) Error() string { return e.cause.Error() }

func (e *transportError) Unwrap() error { return e.cause }

// listUnfinished starts the message of every error that ends a paged read early.
const listUnfinished = "the list did not finish: "

// limitError means a cap or a check ended a read before it finished.
type limitError struct{ message string }

func (e *limitError) Error() string { return e.message }

// call sends one request and decodes a 2xx JSON response into out. A non-2xx
// response comes back as a *drapi.HTTPError that carries the start of the
// response body with the token redacted, which reason turns into the section's
// message. authorize false leaves the token off, for public routes. A 429 or
// 503 answer is sent again up to maxRetries times, after the wait the server
// names in Retry-After or a jittered, growing one.
//
// call does not use drapi.Get, Post or Do, which take no context, so the
// deadline of the whole run could not reach the request, and which follow
// redirects to any origin. It logs the request as they do.
func call(ctx context.Context, method, url string, authorize bool, in, out any) error {
	for attempt := 0; ; attempt++ {
		retryAfter, err := callOnce(ctx, method, url, authorize, in, out)

		delay, again := retryDelay(err, retryAfter, attempt)
		if !again || sleep(ctx, delay) != nil {
			return err
		}
	}
}

// callOnce sends the request once. It also returns the Retry-After header of a
// failed answer, first.
func callOnce(ctx context.Context, method, url string, authorize bool, in, out any) (string, error) {
	req, err := newRequest(ctx, method, url, authorize, in)
	if err != nil {
		return "", err
	}

	log.Debug("Request Info: \n" + config.RedactedReqInfo(req))

	client := drapi.NewHTTPClient(callTimeout)
	client.CheckRedirect = stayOnOrigin

	resp, err := client.Do(req)
	if err != nil {
		return "", &transportError{err}
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.Header.Get("Retry-After"), httpError(resp, url, req.Header.Get("Authorization"))
	}

	return "", decodeJSON(resp.Body, out)
}

// retryDelay says whether a failed attempt is worth another and how long to
// wait first. Only 429 and 503 are: a retry cannot change any other answer, and
// a 502 from the proxy of a switched-off service is a stable state the report
// should show at once.
func retryDelay(err error, retryAfter string, attempt int) (time.Duration, bool) {
	status := statusOf(err)
	if (status != http.StatusTooManyRequests && status != http.StatusServiceUnavailable) || attempt >= maxRetries {
		return 0, false
	}

	if asked, ok := parseRetryAfter(retryAfter, now()); ok {
		return asked, asked <= retryAfterCap
	}

	return jitter(min(retryCap, retryBase<<attempt)), true
}

// parseRetryAfter reads the Retry-After header: a number of seconds or an HTTP
// date. A date in the past means no wait.
func parseRetryAfter(value string, at time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}

	if date, err := http.ParseTime(value); err == nil {
		return max(0, date.Sub(at)), true
	}

	return 0, false
}

// stayOnOrigin refuses a redirect to another scheme, host or port than the
// request began on. Go keeps the Authorization header for a redirect to the
// same hostname, whatever the scheme or port, so the token would reach a
// listener the install did not name.
func stayOnOrigin(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("stopped after too many redirects")
	}

	first := via[0].URL
	if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host {
		return fmt.Errorf("the install redirected to %s://%s, which is not the configured address; set the endpoint to it",
			req.URL.Scheme, req.URL.Host)
	}

	return nil
}

// bearerPattern matches a bearer credential wherever an error page echoes one.
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+\S+`)

// redact removes a bearer credential and the request's own token from s.
func redact(s, authorization string) string {
	s = bearerPattern.ReplaceAllString(s, "Bearer [redacted]")

	if token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer")); token != "" {
		s = strings.ReplaceAll(s, token, "[redacted]")
	}

	return s
}

// httpError turns a non-2xx response into a *drapi.HTTPError whose Body is the
// start of the response with credentials redacted, and whose Detail is the same
// text cleaned for a terminal. The error page of a proxy may echo the request
// headers.
func httpError(resp *http.Response, requestURL, authorization string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, errorReadBytes))
	body := redact(string(raw), authorization)

	if len(body) > errorBodyBytes {
		body = body[:errorBodyBytes]
	}

	return &drapi.HTTPError{StatusCode: resp.StatusCode, URL: requestURL, Detail: clean(body), Body: []byte(body)}
}

// newRequest builds a request with a JSON body when in is set. authorize false
// leaves the token off and sends only the user agent.
func newRequest(ctx context.Context, method, url string, authorize bool, in any) (*http.Request, error) {
	var body io.Reader

	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}

		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if !authorize {
		req.Header.Set("User-Agent", config.GetUserAgentHeader())

		return req, nil
	}

	if err = drapi.AuthorizeRequest(req); err != nil { //nolint:contextcheck // drapi takes no context
		return nil, err
	}

	return req, nil
}

// decodeJSON reads one JSON value from r into out. A response larger than
// maxBodyBytes is refused before it is decoded.
func decodeJSON(r io.Reader, out any) error {
	raw, err := io.ReadAll(io.LimitReader(r, maxBodyBytes+1))
	if err != nil {
		return &transportError{err}
	}

	if len(raw) > maxBodyBytes {
		return &transportError{fmt.Errorf("the response is larger than %d MiB", maxBodyBytes>>20)}
	}

	err = json.NewDecoder(bytes.NewReader(raw)).Decode(out)
	if err == nil {
		return nil
	}

	var syntax *json.SyntaxError

	if errors.As(err, &syntax) || errors.Is(err, io.EOF) {
		return &transportError{fmt.Errorf("the response is not JSON (a login or proxy page?): %w", err)}
	}

	return &transportError{err}
}

// listAll reads a paged list to the end, following each page's next cursor.
// The token goes with every request, so a cursor must stay on the requested
// route of the configured install, and the read stops at maxPages pages or
// maxItems items, or when a cursor repeats.
func listAll[T any](ctx context.Context, firstURL string) ([]T, error) {
	route, err := url.Parse(firstURL)
	if err != nil {
		return nil, err
	}

	var all []T

	seen := map[string]bool{firstURL: true}

	for pages, next := 1, firstURL; next != ""; pages++ {
		if pages > maxPages {
			return nil, &limitError{fmt.Sprintf("%smore than %d pages", listUnfinished, maxPages)}
		}

		var page struct {
			Data []T    `json:"data"`
			Next string `json:"next"`
		}

		if err = call(ctx, http.MethodGet, next, true, nil, &page); err != nil {
			return nil, err
		}

		all = append(all, page.Data...)

		if len(all) > maxItems {
			return nil, &limitError{fmt.Sprintf("%smore than %d items", listUnfinished, maxItems)}
		}

		next = page.Next

		if next == "" {
			break
		}

		if err = checkCursor(route, next, seen); err != nil {
			return nil, err
		}

		seen[next] = true
	}

	return all, nil
}

// checkCursor refuses a next cursor that leaves the configured install, the
// requested route, or carries user information, or that names a page already
// read.
func checkCursor(route *url.URL, next string, seen map[string]bool) error {
	if err := drapi.AssertNextOnSameHost(next); err != nil {
		return fmt.Errorf("%s%w", listUnfinished, err)
	}

	cursor, err := url.Parse(next)
	if err != nil {
		return err
	}

	switch {
	case cursor.User != nil:
		return &limitError{listUnfinished + "the next cursor carries user information"}
	case cursor.Path != route.Path:
		return &limitError{listUnfinished + "the next cursor leaves the route that was requested"}
	case seen[next]:
		return &limitError{listUnfinished + "the next cursor repeats a page already read"}
	}

	return nil
}

// reason words a failed call for a section's message: the status and the
// response body for an HTTP error, the error itself for anything else.
func reason(err error) string {
	var httpErr *drapi.HTTPError

	if !errors.As(err, &httpErr) {
		return clean(err.Error())
	}

	body := clean(string(httpErr.Body))
	if body == "" {
		return fmt.Sprintf("HTTP %d", httpErr.StatusCode)
	}

	return fmt.Sprintf("HTTP %d: %s", httpErr.StatusCode, body)
}

// statusOf returns the HTTP status of a failed call, or zero when no response
// came back.
func statusOf(err error) int {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}

	return 0
}

// isTransport reports whether a call failed to get a usable answer. It is
// false for a failure that never left the machine.
func isTransport(err error) bool {
	var transport *transportError

	return errors.As(err, &transport)
}
