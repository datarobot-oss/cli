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

package workload

import (
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

// installSkipAuth configures viper so drapi.AuthorizeRequest does not
// attempt to verify a token over the network. Mirrors the helper in
// internal/pipeline/transport_test.go.
func installSkipAuth(t *testing.T) {
	t.Helper()

	prevSkip := viperx.GetBool("skip_auth")
	prevTok := viperx.GetString(config.DataRobotAPIKey)

	viperx.Set("skip_auth", true)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	t.Cleanup(func() {
		viperx.Set("skip_auth", prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevTok)
	})
}

// installEndpoint redirects config.GetEndpointURL("/...") to point at the
// given httptest server URL for the duration of the test.
func installEndpoint(t *testing.T, url string) {
	t.Helper()

	prev := viperx.GetString(config.DataRobotURL)

	viperx.Set(config.DataRobotURL, url)

	t.Cleanup(func() {
		viperx.Set(config.DataRobotURL, prev)
	})
}
