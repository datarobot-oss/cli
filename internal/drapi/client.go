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
	"time"
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
