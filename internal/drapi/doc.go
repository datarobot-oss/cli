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

// Package drapi is the CLI's shared HTTP client for every DataRobot API it
// talks to: the Public API, the Workload API, and their sibling FastAPI
// services. A new command that needs to call one of those APIs should build
// on this package rather than constructing its own *http.Client.
//
// # Verbs
//
// Get, Post, Patch, and Delete each send one request and return the raw
// *http.Response; GetJSON, PostJSON, PatchJSON, and DeleteJSON additionally
// decode a JSON response body into a caller-supplied value. Post, Patch, and
// Delete marshal their body argument as JSON — none of the four support
// multipart/form-data; a caller that needs to upload a file builds its own
// *http.Request (see internal/pipeline's doMultipart) and sends it with Do.
//
// # Timeouts
//
// Every verb function takes an optional trailing time.Duration override:
//
//	drapi.Get(url, "info")                   // DefaultClientTimeout (30s)
//	drapi.Get(url, "info", 5*time.Second)     // 5s
//
// Omitting the argument, or passing a value ≤0, falls back to
// DefaultClientTimeout — the same clamp NewHTTPClient applies, so a caller
// that computes a timeout dynamically (e.g. from a config value that could
// legitimately be zero) never has to guard it themselves. A caller that
// builds its own request should call Do(req, timeout) rather than
// NewHTTPClient(timeout).Do(req) directly, for the same reason.
//
// # Errors
//
// A non-2xx response comes back as *HTTPError, which callers unpack with
// errors.As to read StatusCode:
//
//	var httpErr *drapi.HTTPError
//	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
//		...
//	}
//
// DataRobot's APIs don't agree on an error envelope (FastAPI services answer
// {"detail": ...}, the drflask Public API {"message": ...}, a proxy in
// between plain text), so HTTPError carries the response body raw and
// uninterpreted on Body rather than guessing at a shape. HTTPError.Detail
// exists for a caller that has already parsed a message out of Body — it's
// best-effort and caller-populated, not something drapi itself fills in.
// Branch decisions on StatusCode, which the server always sets; use Detail
// for display only. See internal/workload/apiclient for the pattern to
// follow when an API's envelope needs that parsing: it reads Body into a
// Detail-bearing HTTPError while keeping the result unpackable with the same
// errors.As(err, &httpErr) callers already use everywhere else.
package drapi
