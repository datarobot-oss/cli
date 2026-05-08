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
	"net/http"

	"github.com/datarobot/cli/internal/config"
)

// SetAuthHeaders populates Authorization, User-Agent, and (when enabled)
// X-DataRobot-Api-Consumer-Trace on req using the same sources the verb
// helpers use inline. Exposed so callers that build their own *http.Request
// (e.g. multipart streaming uploads in drapi/filesapi) can reuse the
// canonical drapi auth-injection logic instead of re-implementing it.
//
// Reuses the package-level `token` memoization declared in get.go.
func SetAuthHeaders(req *http.Request) error {
	var err error

	if token, err = resolveToken(); err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	if config.IsAPIConsumerTrackingEnabled() {
		req.Header.Set("X-DataRobot-Api-Consumer-Trace", config.GetAPIConsumerTrace())
	}

	return nil
}
