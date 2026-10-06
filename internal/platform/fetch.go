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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// callTimeout bounds one request, so a source that hangs does not hold the
// report back.
const callTimeout = 15 * time.Second

// apiURL builds a URL under the configured install's /api/v2.
func apiURL(path string, query url.Values) (string, error) {
	return drapi.EndpointURL(path, query)
}

// publicURL builds a URL at the configured install's root, where the
// unauthenticated /config route lives.
func publicURL(path string) (string, error) {
	return config.GetEndpointURL(path)
}

// call sends one request and decodes a 2xx JSON response into out. A non-2xx
// response comes back as a *drapi.HTTPError that carries the response body,
// which reason turns into the section's message. authorize false leaves the
// token off, for public routes.
//
// drapi.Get is not used because it drops the body of an error response, and
// the body is what names the cause.
func call(ctx context.Context, method, url string, authorize bool, in, out any) error {
	var body io.Reader

	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}

		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")

	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if authorize {
		if err = drapi.AuthorizeRequest(req); err != nil {
			return err
		}
	} else {
		req.Header.Set("User-Agent", config.GetUserAgentHeader())
	}

	resp, err := drapi.Do(req, callTimeout)
	if err != nil {
		return err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return drapi.ErrFromResp(resp, url)
	}

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(out)
}

// listAll reads a paged list to the end, following each page's next cursor.
// A cursor that leaves the configured install is refused, because the token
// goes with every request.
func listAll[T any](ctx context.Context, firstURL string) ([]T, error) {
	var all []T

	for next := firstURL; next != ""; {
		var page struct {
			Data []T    `json:"data"`
			Next string `json:"next"`
		}

		if err := call(ctx, http.MethodGet, next, true, nil, &page); err != nil {
			return nil, err
		}

		all = append(all, page.Data...)
		next = page.Next

		if next != "" {
			if err := drapi.AssertNextOnSameHost(next); err != nil {
				return nil, err
			}
		}
	}

	return all, nil
}

// reason words a failed call for a section's message: the status and the
// response body for an HTTP error, the error itself for anything else.
func reason(err error) string {
	var httpErr *drapi.HTTPError

	if !errors.As(err, &httpErr) {
		return err.Error()
	}

	body := strings.TrimSpace(string(httpErr.Body))
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

// isTransport reports whether a call failed without an HTTP error status: the
// install is unreachable, the name does not resolve, TLS failed, or the answer
// was not the JSON the route promises.
func isTransport(err error) bool {
	return statusOf(err) == 0
}
