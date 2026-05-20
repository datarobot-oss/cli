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

// This file owns the cross-verb HTTP primitives shared by every DataRobot
// API call: a client constructor, the standard header set, and the cached
// token resolver. Verb-specific helpers (Get/GetJSON in get.go, future
// post.go/patch.go, etc.) reuse these to stay consistent.
//
// HTTPError, the package-level `token` cache, and resolveToken() live in
// get.go for historical reasons and are reused from this file.

package drapi

import (
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
)

// DefaultClientTimeout is the read/write timeout used by NewHTTPClient when
// callers don't specify their own.
const DefaultClientTimeout = 30 * time.Second

// getToken returns the memoized API token, resolving and caching it on first
// use to avoid repeated VerifyToken() round-trips. The underlying `token`
// variable and resolveToken() function are defined in get.go.
func getToken() (string, error) {
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
// future shared-transport tweaks live in one place.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// AuthorizeRequest sets the standard DataRobot API headers on req:
// Authorization (Bearer token), User-Agent, and the optional
// X-DataRobot-Api-Consumer-Trace. The request body is never read, so this
// is safe to call on multipart upload requests.
func AuthorizeRequest(req *http.Request) error {
	bearer, err := getToken()
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	if config.IsAPIConsumerTrackingEnabled() {
		req.Header.Set("X-DataRobot-Api-Consumer-Trace", config.GetAPIConsumerTrace())
	}

	return nil
}
