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
	"context"
	"net/http"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

// token is the memoized API token shared by all outbound request helpers
// (Get, Post, Patch, Delete, SetAuthHeaders). Resolved lazily on first use
// via resolveToken.
var token string

// GetToken returns the current cached API token.
func GetToken() string {
	return token
}

// SetToken sets the cached API token.
func SetToken(value string) {
	token = value
}

// resolveToken returns the API token for outbound requests.
// When --skip-auth (or DATAROBOT_CLI_SKIP_AUTH) is active the value is read
// from viper without contacting the server, so local development against stub
// APIs that don't implement /version/ still works.
func resolveToken() (string, error) {
	if viperx.GetBool("skip_auth") {
		return viperx.GetString(config.DataRobotAPIKey), nil
	}

	return config.GetAPIKey(context.Background())
}

// SetAuthHeaders populates Authorization, User-Agent, and (when enabled)
// X-DataRobot-Api-Consumer-Trace on req. Exposed so callers that build their
// own *http.Request (e.g. multipart streaming uploads in drapi/filesapi) can
// reuse the canonical auth-injection logic instead of re-implementing it.
func SetAuthHeaders(req *http.Request) error {
	if token == "" {
		resolved, err := resolveToken()
		if err != nil {
			return err
		}

		token = resolved
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	if config.IsAPIConsumerTrackingEnabled() {
		req.Header.Set("X-DataRobot-Api-Consumer-Trace", config.GetAPIConsumerTrace())
	}

	return nil
}
