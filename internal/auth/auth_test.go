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

package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setupTestEnvironment creates a temporary environment for testing authentication.
// It sets up a temporary config directory, a mock DataRobot API server, and
// returns a cleanup function to restore the original state.
func setupTestEnvironment(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// So much of the login process depends on the config file location, and
	// we obviously don't want to destroy users' real config files, so we
	// create a temp dir and point HOME there.
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	testutil.SetTestHomeDir(t, tempDir)

	// Without this the developer's own env pair reaches EnsureAuthenticated,
	// which then verifies against their real instance instead of the mock.
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	// Save original callback function.
	originalCallback := APIKeyCallbackFunc

	viperx.Reset()

	// Create mock DataRobot API server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/version/" {
			// Check authorization header.
			auth := r.Header.Get("Authorization")
			if auth == "Bearer valid-token" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"version":"10.0.0"}`))

				return
			}

			if auth == "Bearer expired-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Invalid credentials"}`))

				return
			}

			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")

	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	cleanup := func() {
		server.Close()
		os.RemoveAll(tempDir)
		viperx.Reset()

		APIKeyCallbackFunc = originalCallback
	}

	return server, cleanup
}

func TestEnsureAuthenticated_MissingCredentials(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	// Mock the callback to simulate failure to retrieve API key.
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("simulated authentication failure")
	}

	// EnsureAuthenticated should detect missing credentials.
	result := EnsureAuthenticated(context.Background())
	assert.False(t, result, "Expected EnsureAuthenticated to return false with missing credentials")

	// Verify the URL is properly configured.
	baseURL := config.GetBaseURL()
	assert.Equal(t, server.URL, baseURL, "Expected base URL to be set from test server")
}

func TestEnsureAuthenticated_ExpiredCredentials(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "expired-token")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	apiKey, _ := config.GetAPIKey(context.Background())
	assert.Empty(t, apiKey, "Expected GetAPIKey to return empty string for expired token")

	// Mock the callback to simulate failure to refresh expired credentials.
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("simulated authentication failure")
	}

	result := EnsureAuthenticated(context.Background())
	assert.False(t, result, "Expected EnsureAuthenticated to return false with expired credentials")
}

func TestEnsureAuthenticated_ValidCredentials(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "valid-token")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	apiKey, _ := config.GetAPIKey(context.Background())
	assert.Equal(t, "valid-token", apiKey, "Expected GetAPIKey to return valid token")

	result := EnsureAuthenticated(context.Background())
	assert.True(t, result, "Expected EnsureAuthenticated to return true with valid credentials")
}

func TestEnsureAuthenticated_ValidEnvironmentToken(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	os.Setenv("DATAROBOT_ENDPOINT", server.URL+"/api/v2")
	os.Setenv("DATAROBOT_API_TOKEN", "valid-token")

	apiKey, _ := config.GetAPIKey(context.Background())
	assert.Empty(t, apiKey, "Expected GetAPIKey before EnsureAuthenticated to return empty string")

	result := EnsureAuthenticated(context.Background())
	assert.True(t, result, "Expected EnsureAuthenticated to return true with valid environment credentials")

	apiKey, _ = config.GetAPIKey(context.Background())
	assert.Equal(t, "valid-token", apiKey, "Expected GetAPIKey after EnsureAuthenticated to return valid token")
}

// TestEnsureAuthenticated_EnvInvalidStoredValid is the regression test for the
// silent-fallback bug: a complete pair of environment credentials that fails
// verification must fail the command, never silently substitute the stored
// profile (which is valid here and would have made the old code return true).
func TestEnsureAuthenticated_EnvInvalidStoredValid(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "valid-token")
	t.Setenv("DATAROBOT_ENDPOINT", server.URL+"/api/v2")
	t.Setenv("DATAROBOT_API_TOKEN", "expired-token")

	// The login flow must not start either; fail the test if it does.
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		t.Error("login flow must not start when explicit env credentials fail")

		return "", errors.New("unexpected login flow")
	}

	result := EnsureAuthenticated(context.Background())
	assert.False(t, result,
		"Expected EnsureAuthenticated to fail instead of falling back to the valid stored profile")
}

// TestEnsureAuthenticated_EnvMalformedEndpointStoredValid covers the malformed
// endpoint variant of the same rule (e.g. a quoted URL from running
// `$(dr auth export)` without eval).
func TestEnsureAuthenticated_EnvMalformedEndpointStoredValid(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "valid-token")
	t.Setenv("DATAROBOT_ENDPOINT", `"`+server.URL+`/api/v2"`)
	t.Setenv("DATAROBOT_API_TOKEN", "valid-token")

	result := EnsureAuthenticated(context.Background())
	assert.False(t, result,
		"Expected EnsureAuthenticated to fail on a malformed endpoint instead of using the stored profile")
}

func TestReportEnvCredentialsError(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: "https://app.example.com/api/v2", Token: "some-token"}
		ReportEnvCredentialsError(&buf, creds, context.DeadlineExceeded)

		assert.Contains(t, buf.String(), "timed out")
		assert.Contains(t, buf.String(), "https://app.example.com")
	})

	t.Run("malformed endpoint", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: `"https://app.example.com/api/v2"`, Token: "some-token"}
		ReportEnvCredentialsError(&buf, creds, errors.New("invalid token"))

		assert.Contains(t, buf.String(), "DATAROBOT_ENDPOINT environment variable is invalid")
		assert.NotContains(t, buf.String(), "DATAROBOT_API_TOKEN environment variable is invalid or expired")
	})

	t.Run("invalid token", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: "https://app.example.com/api/v2", Token: "bad-token"}
		ReportEnvCredentialsError(&buf, creds, errors.New("invalid token"))

		assert.Contains(t, buf.String(), "DATAROBOT_API_TOKEN environment variable is invalid or expired")
		assert.Contains(t, buf.String(), "unset DATAROBOT_API_TOKEN")
	})

	t.Run("scheme-less endpoint is an endpoint problem, not transport", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: "app.example.com/api/v2", Token: "some-token"}
		dialErr := &url.Error{
			Op:  "Get",
			URL: creds.Endpoint + "/version/",
			Err: errors.New(`unsupported protocol scheme ""`),
		}
		ReportEnvCredentialsError(&buf, creds, dialErr)

		assert.Contains(t, buf.String(), "missing URL scheme")
		assert.NotContains(t, buf.String(), "Could not connect")
	})

	t.Run("raw parse failure is an endpoint problem, not transport", func(t *testing.T) {
		var buf bytes.Buffer

		// Leading whitespace fails VerifyToken's raw url.Parse but survives
		// ValidateEndpoint, which trims before parsing.
		creds := &EnvCredentials{Endpoint: " https://app.example.com/api/v2", Token: "some-token"}
		parseErr := &url.Error{
			Op:  "parse",
			URL: creds.Endpoint,
			Err: errors.New("first path segment in URL cannot contain colon"),
		}
		ReportEnvCredentialsError(&buf, creds, parseErr)

		assert.Contains(t, buf.String(), "environment variable is invalid")
		assert.NotContains(t, buf.String(), "Could not connect")
	})

	t.Run("unreachable endpoint is not a token problem", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: "https://app.example.com/api/v2", Token: "valid-but-never-judged"}
		transportErr := &url.Error{
			Op:  "Get",
			URL: creds.Endpoint + "/version/",
			Err: errors.New("dial tcp 203.0.113.1:443: connect: connection refused"),
		}
		ReportEnvCredentialsError(&buf, creds, transportErr)

		assert.Contains(t, buf.String(), "Could not connect to")
		assert.Contains(t, buf.String(), "connection refused")
		assert.NotContains(t, buf.String(), "unset DATAROBOT_API_TOKEN")
	})

	t.Run("names DATAROBOT_API_ENDPOINT when the endpoint came from the fallback var", func(t *testing.T) {
		var buf bytes.Buffer

		t.Setenv("DATAROBOT_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_ENDPOINT", `"https://app.example.com/api/v2"`)
		t.Setenv("DATAROBOT_API_TOKEN", "some-token")

		creds := GetEnvCredentials()
		ReportEnvCredentialsError(&buf, &creds, errors.New("parse error"))

		assert.Contains(t, buf.String(), "DATAROBOT_API_ENDPOINT environment variable is invalid")
		assert.NotContains(t, buf.String(), "DATAROBOT_ENDPOINT environment variable is invalid")
	})

	t.Run("unsupported scheme is an endpoint problem, not transport", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: "ftp://app.example.com/api/v2", Token: "some-token"}
		dialErr := &url.Error{
			Op:  "Get",
			URL: creds.Endpoint + "/version/",
			Err: errors.New(`unsupported protocol scheme "ftp"`),
		}
		ReportEnvCredentialsError(&buf, creds, dialErr)

		assert.Contains(t, buf.String(), `unsupported URL scheme "ftp"`)
		assert.NotContains(t, buf.String(), "Could not connect")
	})

	// Only 401 and 403 may blame the token; every other status is the server
	// failing to answer the version check.
	creds := &EnvCredentials{Endpoint: "https://app.example.com/api/v2", Token: "some-token"}

	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprintf("%d blames the token", status), func(t *testing.T) {
			var buf bytes.Buffer

			ReportEnvCredentialsError(&buf, creds, &config.HTTPStatusError{StatusCode: status})

			assert.Contains(t, buf.String(), "DATAROBOT_API_TOKEN environment variable is invalid or expired")
			assert.Contains(t, buf.String(), "unset DATAROBOT_API_TOKEN")
		})
	}

	for _, status := range []int{
		http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError,
	} {
		t.Run(fmt.Sprintf("%d blames the instance", status), func(t *testing.T) {
			var buf bytes.Buffer

			ReportEnvCredentialsError(&buf, creds, &config.HTTPStatusError{StatusCode: status})

			assert.Contains(t, buf.String(), fmt.Sprintf("answered HTTP %d", status))
			assert.Contains(t, buf.String(), "https://app.example.com")
			assert.NotContains(t, buf.String(), "unset DATAROBOT_API_TOKEN")
		})
	}
}

func TestReportUnjudged(t *testing.T) {
	const endpoint = "https://app.example.com/api/v2"

	cases := []struct {
		name        string
		endpoint    string
		err         error
		wantHandled bool
		contains    string
	}{
		{"timeout", endpoint, context.DeadlineExceeded, true, "timed out"},
		{"404", endpoint, &config.HTTPStatusError{StatusCode: http.StatusNotFound}, true, "answered HTTP 404"},
		{"429", endpoint, &config.HTTPStatusError{StatusCode: http.StatusTooManyRequests}, true, "answered HTTP 429"},
		{"503", endpoint, &config.HTTPStatusError{StatusCode: http.StatusServiceUnavailable}, true, "answered HTTP 503"},
		{"401 is a verdict", endpoint, &config.HTTPStatusError{StatusCode: http.StatusUnauthorized}, false, ""},
		{"403 is a verdict", endpoint, &config.HTTPStatusError{StatusCode: http.StatusForbidden}, false, ""},
		// An absent token carries no status, so it must read as a verdict and
		// let the caller start the login flow.
		{"empty token is a verdict", endpoint, errors.New("empty token"), false, ""},
		{
			"transport", endpoint,
			&url.Error{Op: "Get", URL: endpoint, Err: errors.New("connection refused")},
			true, "Could not connect to",
		},
		{
			"raw parse failure", " " + endpoint,
			&url.Error{Op: "parse", URL: " " + endpoint, Err: errors.New("cannot contain colon")},
			true, "is invalid",
		},
		// Reported as the missing scheme it is, not as a failure to reach the
		// https host SchemeHostOnly would have invented.
		{
			"bare host", "app.example.com",
			&url.Error{Op: "Get", URL: "app.example.com", Err: errors.New(`unsupported protocol scheme ""`)},
			true, "missing URL scheme",
		},
		{
			"ftp scheme", "ftp://app.example.com",
			&url.Error{Op: "Get", URL: "ftp://app.example.com", Err: errors.New(`unsupported protocol scheme "ftp"`)},
			true, `unsupported URL scheme "ftp"`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer

			handled := ReportUnjudged(&buf, c.endpoint, StoredEndpointName, c.err)

			assert.Equal(t, c.wantHandled, handled)

			if c.wantHandled {
				assert.Contains(t, buf.String(), c.contains)
			} else {
				assert.Empty(t, buf.String(), "a rejected credential is the caller's message to write")
			}
		})
	}
}

// TestEndpointUserinfoRedacted covers DATAROBOT_ENDPOINT=https://user:pass@host,
// which put the password on stderr on every message naming the endpoint.
func TestEndpointUserinfoRedacted(t *testing.T) {
	const withPassword = "https://user:s3cr3t@app.example.com/api/v2"

	errs := map[string]error{
		"timeout":   context.DeadlineExceeded,
		"transport": &url.Error{Op: "Get", URL: withPassword, Err: errors.New("connection refused")},
		"status":    &config.HTTPStatusError{StatusCode: http.StatusServiceUnavailable},
	}

	for name, err := range errs {
		t.Run("ReportUnjudged "+name, func(t *testing.T) {
			var buf bytes.Buffer

			require.True(t, ReportUnjudged(&buf, withPassword, StoredEndpointName, err))
			assert.NotContains(t, buf.String(), "s3cr3t")
			assert.Contains(t, buf.String(), "xxxxx")
		})

		t.Run("ReportEnvCredentialsError "+name, func(t *testing.T) {
			var buf bytes.Buffer

			creds := &EnvCredentials{Endpoint: withPassword, Token: "some-token"}
			ReportEnvCredentialsError(&buf, creds, err)

			assert.NotContains(t, buf.String(), "s3cr3t")
			assert.Contains(t, buf.String(), "xxxxx")
		})
	}
}

// TestUnusableEndpointHidesUserinfo covers url.Parse embedding the endpoint in
// its own error text, which carried the password past every other redaction.
func TestUnusableEndpointHidesUserinfo(t *testing.T) {
	// Quoted, so SchemeHostOnly's url.Parse rejects it outright.
	const quoted = `"https://user:s3cr3t@app.example.com/api/v2"`

	t.Run("ReportEnvCredentialsError", func(t *testing.T) {
		var buf bytes.Buffer

		creds := &EnvCredentials{Endpoint: quoted, Token: "some-token"}
		ReportEnvCredentialsError(&buf, creds, errors.New("invalid token"))

		assert.Contains(t, buf.String(), "DATAROBOT_ENDPOINT environment variable is invalid")
		assert.NotContains(t, buf.String(), "s3cr3t")
	})

	t.Run("ReportUnjudged", func(t *testing.T) {
		var buf bytes.Buffer

		require.True(t, ReportUnjudged(&buf, quoted, StoredEndpointName, errors.New("invalid token")))
		assert.NotContains(t, buf.String(), "s3cr3t")
	})

	t.Run("redacted fails closed on a value url.Parse rejects", func(t *testing.T) {
		assert.NotContains(t, redacted(quoted), "s3cr3t")
		assert.NotContains(t, hostOrEndpoint(quoted), "s3cr3t")
	})
}

// TestAuthCallbackURLDropsUserinfo covers the sign-in link, which is printed to
// the user and so carried the endpoint's password into the terminal.
func TestAuthCallbackURLDropsUserinfo(t *testing.T) {
	got := AuthCallbackURL("https://user:s3cr3t@app.example.com")

	assert.NotContains(t, got, "s3cr3t")
	assert.NotContains(t, got, "user:")
	assert.Equal(t, "https://app.example.com/account/developer-tools?cliRedirect=true", got)
}

// TestReportStoredProfileNotUsedRedacts covers the refusal line printed directly
// after the classified error, which named the endpoint without redacting it.
func TestReportStoredProfileNotUsedRedacts(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	var buf bytes.Buffer

	creds := &EnvCredentials{Endpoint: "https://user:s3cr3t@requested.example.com/api/v2", Token: "bad-token"}
	reportStoredProfileNotUsed(&buf, creds)

	assert.Contains(t, buf.String(), "not falling back to the stored profile")
	assert.NotContains(t, buf.String(), "s3cr3t")
}

// TestEnsureAuthenticated_NoTokenBadEndpoint is the lockout regression test: an
// endpoint the CLI cannot dial must not withhold the login flow that fixes it.
func TestEnsureAuthenticated_NoTokenBadEndpoint(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotURL, "app.datarobot.invalid")
	viperx.Set(config.DataRobotAPIKey, "")

	loginStarted := false
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		loginStarted = true

		return "fresh-token", nil
	}

	EnsureAuthenticated(context.Background())

	assert.True(t, loginStarted, "Expected the login flow to open when no token is stored")
}

// TestEnsureAuthenticated_StoredProfileServerError is the regression test for
// the token-wiping bug: a 503 never judged the token, so it must survive.
func TestEnsureAuthenticated_StoredProfileServerError(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "valid-token")

	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		t.Error("login flow must not start when the instance never judged the token")

		return "", errors.New("unexpected login flow")
	}

	result := EnsureAuthenticated(context.Background())

	assert.False(t, result, "Expected EnsureAuthenticated to fail on a 503 from the stored profile")
	assert.Equal(t, "valid-token", viperx.GetString(config.DataRobotAPIKey),
		"Expected the stored token to survive a status the instance never judged it with")
}

// TestEnsureAuthenticated_StoredProfileRejected proves the 503 change did not
// disable the login flow for a token the instance actually rejected.
func TestEnsureAuthenticated_StoredProfileRejected(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "expired-token")

	loginStarted := false
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		loginStarted = true

		return "fresh-token", nil
	}

	result := EnsureAuthenticated(context.Background())

	assert.True(t, result)
	assert.True(t, loginStarted, "Expected a 401 to still open the login flow")
	assert.Equal(t, "fresh-token", viperx.GetString(config.DataRobotAPIKey))
}

// TestEnsureAuthenticated_NoStoredToken keeps a fresh install working: an absent
// token carries no status, so it must still reach the login flow.
func TestEnsureAuthenticated_NoStoredToken(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	viperx.Set(config.DataRobotAPIKey, "")

	loginStarted := false
	APIKeyCallbackFunc = func(_ context.Context, _ string) (string, error) {
		loginStarted = true

		return "fresh-token", nil
	}

	result := EnsureAuthenticated(context.Background())

	assert.True(t, result)
	assert.True(t, loginStarted, "Expected an absent stored token to open the login flow")
}

// TestEnsureAuthenticated_EnvUnreachableStoredValid proves VerifyToken's
// transport errors really surface as *url.Error and classify as
// could-not-connect end to end, still failing instead of using the stored
// profile.
func TestEnsureAuthenticated_EnvUnreachableStoredValid(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// A second server, closed immediately, donates a port that refuses
	// connections.
	deadServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	deadServer.Close()

	viperx.Set(config.DataRobotAPIKey, "valid-token")
	t.Setenv("DATAROBOT_ENDPOINT", deadServer.URL+"/api/v2")
	t.Setenv("DATAROBOT_API_TOKEN", "valid-token")

	result := EnsureAuthenticated(context.Background())
	assert.False(t, result,
		"Expected EnsureAuthenticated to fail on an unreachable endpoint instead of using the stored profile")
}

func TestReportStoredProfileNotUsed(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	creds := &EnvCredentials{Endpoint: "https://requested.example.com/api/v2", Token: "bad-token"}

	t.Run("names both endpoints when a stored profile exists", func(t *testing.T) {
		var buf bytes.Buffer

		reportStoredProfileNotUsed(&buf, creds)

		assert.Contains(t, buf.String(), "https://requested.example.com")
		assert.Contains(t, buf.String(), "not falling back to the stored profile")
	})

	t.Run("silent without a stored profile", func(t *testing.T) {
		var buf bytes.Buffer

		viperx.Set(config.DataRobotURL, "")
		reportStoredProfileNotUsed(&buf, creds)

		assert.Empty(t, buf.String())
	})
}

func TestEnsureAuthenticated_SkipAuth(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Set the skip-auth flag to true
	viperx.Set(config.SkipAuthKey, true)

	// Don't set any credentials
	viperx.Set(config.DataRobotAPIKey, "")

	existingToken := os.Getenv("DATAROBOT_API_TOKEN")

	os.Unsetenv("DATAROBOT_API_TOKEN")

	defer os.Setenv("DATAROBOT_API_TOKEN", existingToken)

	result := EnsureAuthenticated(context.Background())
	assert.True(t, result, "Expected EnsureAuthenticated to return true when skip_auth is enabled via config")

	os.Setenv("DATAROBOT_CLI_SKIP_AUTH", "true")

	defer os.Unsetenv("DATAROBOT_CLI_SKIP_AUTH")

	result = EnsureAuthenticated(context.Background())
	assert.True(t, result, "Expected EnsureAuthenticated to return true when skip_auth is enabled via environment variable")
}

func TestEnsureAuthenticated_DefaultUserAgent(t *testing.T) {
	var capturedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/version/" {
			capturedUserAgent = r.Header.Get("User-Agent")

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"10.0.0"}`))
		}
	}))
	defer server.Close()

	viperx.Reset()
	viperx.Set(config.DataRobotURL, server.URL+"/api/v2")
	viperx.Set(config.DataRobotAPIKey, "valid-token")

	os.Unsetenv("DATAROBOT_ENDPOINT")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	result := EnsureAuthenticated(context.Background())
	assert.True(t, result)

	expectedUserAgent := "DataRobot CLI version: dev"
	assert.Equal(t, expectedUserAgent, capturedUserAgent,
		"Expected default User-Agent for normal CLI authentication")
}

func TestEnsureAuthenticated_NoURL(t *testing.T) {
	// Create temp directory for test config.
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	testutil.SetTestHomeDir(t, tempDir)

	viperx.Reset()

	os.Unsetenv("DATAROBOT_ENDPOINT")
	os.Unsetenv("DATAROBOT_API_TOKEN")
	viperx.Set(config.DataRobotURL, "")

	baseURL := config.GetBaseURL()
	assert.Empty(t, baseURL, "Expected GetBaseURL to return empty string")

	result := EnsureAuthenticated(context.Background())
	assert.False(t, result, "Expected EnsureAuthenticated to return false without configured URL")
}

func TestConfig_WriteAndRead(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	configDir := filepath.Join(homeDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFilePath := filepath.Join(configDir, "drconfig.yaml")

	// Write the config file directly rather than going through viper's
	// (forbidden) WriteConfig. The production code path persists via
	// config.UpdateConfigFile; this test only needs a seeded fixture.
	fixture := map[string]any{config.DataRobotAPIKey: "test-token"}

	data, err := yaml.Marshal(fixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configFilePath, data, 0o600))

	viperx.Reset()
	viperx.SetConfigFile(configFilePath)

	err = viperx.ReadInConfig()
	require.NoError(t, err)

	token := viperx.GetString(config.DataRobotAPIKey)
	assert.Equal(t, "test-token", token, "Expected token to be persisted and read back")
}

func TestConfig_ConfigFilePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	testutil.SetTestHomeDir(t, tempDir)

	viperx.Reset()

	err = config.CreateConfigFileDirIfNotExists()
	require.NoError(t, err)

	// Verify config file was created in correct location.
	expectedPath := filepath.Join(tempDir, ".config", "datarobot", "drconfig.yaml")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "Expected config file to exist at %s", expectedPath)
}

func TestGetEnvCredentials(t *testing.T) {
	t.Run("prefers DATAROBOT_ENDPOINT over DATAROBOT_API_ENDPOINT", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "https://primary.example.com")
		t.Setenv("DATAROBOT_API_ENDPOINT", "https://fallback.example.com")
		t.Setenv("DATAROBOT_API_TOKEN", "test-token")

		creds := GetEnvCredentials()

		assert.Equal(t, "https://primary.example.com", creds.Endpoint)
		assert.Equal(t, "test-token", creds.Token)
	})

	t.Run("falls back to DATAROBOT_API_ENDPOINT", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_ENDPOINT", "https://fallback.example.com")
		t.Setenv("DATAROBOT_API_TOKEN", "test-token")

		creds := GetEnvCredentials()

		assert.Equal(t, "https://fallback.example.com", creds.Endpoint)
		assert.Equal(t, "test-token", creds.Token)
	})

	t.Run("returns empty when no env vars set", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_TOKEN", "")

		creds := GetEnvCredentials()

		assert.Empty(t, creds.Endpoint)
		assert.Empty(t, creds.Token)
	})

	t.Run("preserves surrounding quotes verbatim", func(t *testing.T) {
		// GetEnvCredentials must NOT strip quotes. The DataRobot PySDK and RSDK
		// read these variables verbatim, so stripping here would let the CLI
		// silently succeed where other SDKs fail.
		t.Setenv("DATAROBOT_ENDPOINT", "'https://staging.datarobot.com/api/v2'")
		t.Setenv("DATAROBOT_API_TOKEN", "'secret-token'")

		creds := GetEnvCredentials()

		assert.Equal(t, "'https://staging.datarobot.com/api/v2'", creds.Endpoint)
		assert.Equal(t, "'secret-token'", creds.Token)
	})
}

func TestValidateEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"empty returns nil", "", false},
		{"valid url", "https://app.datarobot.com/api/v2", false},
		{"bare host", "app.datarobot.com", false},
		{"single quoted", "'https://staging.datarobot.com/api/v2'", true},
		{"double quoted", `"https://staging.datarobot.com/api/v2"`, true},
		{"mismatched quotes", `'https://staging.datarobot.com/api/v2"`, true},
		{"plain http", "http://localhost:8080/api/v2", false},
		{"uppercase scheme", "HTTPS://app.datarobot.com/api/v2", false},
		{"ftp scheme", "ftp://app.datarobot.com/api/v2", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateEndpoint(c.endpoint)
			if c.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyEnvCredentials(t *testing.T) {
	t.Run("returns error when env vars not set", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_TOKEN", "")

		creds, err := VerifyEnvCredentials(context.Background())

		require.Error(t, err)
		require.ErrorIs(t, err, ErrEnvCredentialsNotSet)
		assert.NotNil(t, creds)
	})

	t.Run("returns error when only endpoint set", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "https://example.com")
		t.Setenv("DATAROBOT_API_TOKEN", "")

		creds, err := VerifyEnvCredentials(context.Background())

		require.Error(t, err)
		require.ErrorIs(t, err, ErrEnvCredentialsNotSet)
		assert.Equal(t, "https://example.com", creds.Endpoint)
	})

	t.Run("returns error when only token set", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_ENDPOINT", "")
		t.Setenv("DATAROBOT_API_TOKEN", "some-token")

		creds, err := VerifyEnvCredentials(context.Background())

		require.Error(t, err)
		require.ErrorIs(t, err, ErrEnvCredentialsNotSet)
		assert.Equal(t, "some-token", creds.Token)
	})

	t.Run("returns error for invalid token", func(t *testing.T) {
		server, cleanup := setupTestEnvironment(t)
		defer cleanup()

		t.Setenv("DATAROBOT_ENDPOINT", server.URL+"/api/v2")
		t.Setenv("DATAROBOT_API_TOKEN", "invalid-token")

		creds, err := VerifyEnvCredentials(context.Background())

		require.Error(t, err)
		require.NotErrorIs(t, err, ErrEnvCredentialsNotSet)
		assert.Equal(t, server.URL+"/api/v2", creds.Endpoint)
	})

	t.Run("returns nil error for valid credentials", func(t *testing.T) {
		server, cleanup := setupTestEnvironment(t)
		defer cleanup()

		t.Setenv("DATAROBOT_ENDPOINT", server.URL+"/api/v2")
		t.Setenv("DATAROBOT_API_TOKEN", "valid-token")

		creds, err := VerifyEnvCredentials(context.Background())

		require.NoError(t, err)
		assert.Equal(t, server.URL+"/api/v2", creds.Endpoint)
		assert.Equal(t, "valid-token", creds.Token)
	})
}
