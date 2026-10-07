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

package drapi

import (
	"fmt"
	"net/url"

	"github.com/datarobot/cli/internal/config"
)

// AssertNextOnSameHost rejects pagination cursors that switch scheme or
// host away from the configured API base. drapi attaches the bearer
// token to whatever URL it gets, so a compromised or buggy server
// response that sets Next to an attacker-controlled origin would leak
// credentials on the next page request.
func AssertNextOnSameHost(rawNextURL string) error {
	next, err := url.Parse(rawNextURL)
	if err != nil {
		return fmt.Errorf("pagination: parse Next URL: %w", err)
	}

	matches, err := URLMatchesConfiguredBase(rawNextURL)
	if err != nil {
		return fmt.Errorf("pagination: %w", err)
	}

	if !matches {
		return fmt.Errorf("pagination: Next URL host %q does not match API base %q", next.Host, config.GetBaseURL())
	}

	return nil
}

// NextPage returns the cursor for the page after the current one, or "" when
// there is none. It is the shared "no Next, else same-host check, then advance"
// step every paged listing repeats: drapi attaches the bearer token to whatever
// URL it is given, so a Next pointing at another host is refused here rather than
// followed (see AssertNextOnSameHost). Callers still decide when an otherwise
// valid Next should not be followed — an empty page, a bound reached — because
// that is about the scan, not the cursor.
func NextPage(next string) (string, error) {
	if next == "" {
		return "", nil
	}

	if err := AssertNextOnSameHost(next); err != nil {
		return "", err
	}

	return next, nil
}
