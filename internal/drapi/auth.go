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
