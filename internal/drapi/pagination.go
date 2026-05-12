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

	base, err := url.Parse(config.GetBaseURL())
	if err != nil {
		return fmt.Errorf("pagination: parse API base URL: %w", err)
	}

	if next.Scheme != base.Scheme || next.Host != base.Host {
		return fmt.Errorf("pagination: Next URL host %q does not match API base host %q", next.Host, base.Host)
	}

	return nil
}
