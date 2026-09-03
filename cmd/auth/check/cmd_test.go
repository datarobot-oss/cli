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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/spf13/cobra"
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

// The '.env' leg consumes VerifyToken's error as an opaque non-nil; the typed
// *config.HTTPStatusError must leave its message and verdict unchanged.
func TestVerifyDotenvToken_StatusErrorKeepsMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	orig := os.Stdout

	t.Cleanup(func() { os.Stdout = orig })

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	valid := verifyDotenvToken(server.URL+"/api/v2", "expired-token")

	os.Stdout = orig

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	require.False(t, valid)
	assert.Contains(t, string(out), "DATAROBOT_API_TOKEN in '.env' is invalid or expired")
}

// skip-auth must short-circuit RunE with no error and no credential check, so a
// no-auth stack (DR Lite) passes the `task dev` auth-check gate.
func TestRunE_SkipAuthShortCircuits(t *testing.T) {
	viperx.Set(config.SkipAuthKey, true)
	t.Cleanup(func() { viperx.Set(config.SkipAuthKey, false) })
	t.Setenv("DATAROBOT_API_TOKEN", "")
	t.Setenv("DATAROBOT_ENDPOINT", "")

	orig := os.Stdout
	t.Cleanup(func() { os.Stdout = orig })

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	runErr := RunE(&cobra.Command{}, nil)

	os.Stdout = orig
	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	require.NoError(t, runErr)
	assert.Contains(t, string(out), "Authentication checks skipped")
}
