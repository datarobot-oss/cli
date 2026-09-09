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

// This file owns the HTTP client constructor, the cached token resolver, and
// the DefaultClientTimeout constant shared by every verb helper (get.go,
// post.go, patch.go, delete.go). AuthorizeRequest lives in auth.go.
//
// HTTPError, the package-level `token` cache, and resolveToken() live in
// get.go for historical reasons and are reused from this file.

package drapi

import (
	"net/http"
	"sync"
	"time"
)

// DefaultClientTimeout is the read/write timeout used by NewHTTPClient when
// callers don't specify their own.
const DefaultClientTimeout = 30 * time.Second

// tokenMu guards the `token` global. See its declaration in get.go.
var tokenMu sync.Mutex

// getToken returns the memoized API token, resolving and caching it on first
// use to avoid repeated VerifyToken() round-trips. The lock is held across
// resolveToken so concurrent callers share one round-trip instead of each
// making their own. A failed resolve is not cached, so a later call retries.
// The `token` variable and resolveToken() are defined in get.go.
func getToken() (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if token != "" {
		return token, nil
	}

	resolved, err := resolveToken()
	if err != nil {
		return "", err
	}

	token = resolved

	return token, nil
}

// NewHTTPClient returns an *http.Client preconfigured with the given timeout.
// Use this in place of constructing &http.Client{...} inline so timeouts and
// future shared-transport tweaks live in one place. A non-positive timeout
// (the zero value included, so an omitted variadic override falls through
// cleanly) clamps to DefaultClientTimeout. Passing 0 straight to http.Client
// would otherwise mean "no timeout" — silently unbounded, not "use the
// default" — since that's how net/http itself reads a zero Timeout.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultClientTimeout
	}

	return &http.Client{Timeout: timeout}
}

// Do sends req using a client built by NewHTTPClient(timeout). It exists for
// callers that must build their own *http.Request — a multipart body, or a
// caller with its own error-decoding shape — so they get the same
// clamp-then-construct behavior as Get/Post/Patch/Delete without hand-rolling
// NewHTTPClient(t).Do(req) themselves.
func Do(req *http.Request, timeout time.Duration) (*http.Response, error) {
	// req is caller-built, same as every other verb function's internally
	// built request in this file — always against a DataRobot endpoint
	// resolved via config.GetEndpointURL/drapi.EndpointURL, never arbitrary
	// user- or network-supplied input, so this isn't an SSRF sink.
	return NewHTTPClient(timeout).Do(req) //nolint:gosec // req comes from a caller-built DataRobot endpoint URL, not external input
}
