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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
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

// The stored-profile leg blames the token only on a real 401; 403 reports the
// account lacking access, and other statuses blame the instance.
func TestCheckCLICredentials_ClassifiesStoredProfileStatus(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		wantContains    string
		wantNotContains string
	}{
		{"401 blames the token", http.StatusUnauthorized, "No valid API key found", ""},
		{"403 reports lacking access", http.StatusForbidden, "lacks API access", "No valid API key found"},
		{"503 blames the instance", http.StatusServiceUnavailable, "answered HTTP 503", "No valid API key found"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
			}))
			t.Cleanup(server.Close)

			t.Setenv("DATAROBOT_ENDPOINT", "")
			t.Setenv("DATAROBOT_API_ENDPOINT", "")
			t.Setenv("DATAROBOT_API_TOKEN", "")

			viperx.Reset()
			viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
			viperx.Set(config.DataRobotAPIKey, "stored-token")
			t.Cleanup(viperx.Reset)

			var buf bytes.Buffer

			valid := checkCLICredentials(&buf)

			require.False(t, valid)
			assert.Contains(t, buf.String(), c.wantContains)

			if c.wantNotContains != "" {
				assert.NotContains(t, buf.String(), c.wantNotContains)
			}
		})
	}
}

// The '.env' leg used to blame the token for every non-200; now 401 blames the
// token, 403 reports lacking access, and other statuses blame the instance.
func TestVerifyDotenvToken_ClassifiesStatus(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		wantContains    string
		wantNotContains string
	}{
		{"401 blames the token", http.StatusUnauthorized, "DATAROBOT_API_TOKEN in '.env' is invalid or expired", ""},
		{"403 reports lacking access", http.StatusForbidden, "lacks API access", "is invalid or expired"},
		{"404 blames the instance", http.StatusNotFound, "answered HTTP 404", "is invalid or expired"},
		{"503 blames the instance", http.StatusServiceUnavailable, "answered HTTP 503", "is invalid or expired"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
			}))
			t.Cleanup(server.Close)

			var buf bytes.Buffer

			valid := verifyDotenvToken(&buf, server.URL+"/api/v2", "some-token")

			require.False(t, valid)
			assert.Contains(t, buf.String(), c.wantContains)

			if c.wantNotContains != "" {
				assert.NotContains(t, buf.String(), c.wantNotContains)
			}
		})
	}
}
