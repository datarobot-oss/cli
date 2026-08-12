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

package check

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCLICredentials_QuotedEndpointNamesEndpointNotToken(t *testing.T) {
	// Reproduces the `$(dr auth export)` footgun: DATAROBOT_ENDPOINT carries
	// literal surrounding quotes. config.VerifyToken fails fast at url.Parse
	// (no network), so checkCLICredentials must report the endpoint as invalid
	// rather than misattributing the failure to the token.
	t.Setenv("DATAROBOT_ENDPOINT", "'https://staging.datarobot.com/api/v2'")
	t.Setenv("DATAROBOT_API_TOKEN", "dummy-token")

	var buf bytes.Buffer

	valid := checkCLICredentials(&buf)

	require.False(t, valid)
	assert.Contains(t, buf.String(), "DATAROBOT_ENDPOINT environment variable is invalid")
	assert.NotContains(t, buf.String(), "DATAROBOT_API_TOKEN environment variable is invalid or expired")
}
