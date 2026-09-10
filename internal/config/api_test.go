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

package config

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

type APITestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *APITestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-api-test")
	suite.tempDir = dir
	suite.T().Setenv("HOME", suite.tempDir)
	testutil.SetXDGEnv(suite.T(), "XDG_CONFIG_HOME", "")
	viper.Reset()
}

func (suite *APITestSuite) TestSetURLToConfig() {
	tests := []struct {
		name        string
		input       string
		expectedURL string
		expectError bool
	}{
		{
			name:        "full URL with API suffix",
			input:       "https://app.datarobot.com/api/v2",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "base URL without suffix",
			input:       "https://app.datarobot.com",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "bare hostname defaults to https",
			input:       "app.datarobot.com",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "bare hostname with path gets trimmed and defaults to https",
			input:       "app.datarobot.com/api/v2",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 1 resolves to app.datarobot.com",
			input:       "1",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 2 resolves to app.eu.datarobot.com",
			input:       "2",
			expectedURL: "https://app.eu.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 3 resolves to app.jp.datarobot.com",
			input:       "3",
			expectedURL: "https://app.jp.datarobot.com/api/v2",
		},
		{
			name:        "URL with trailing path gets trimmed to host",
			input:       "https://custom.example.com/some/path",
			expectedURL: "https://custom.example.com/api/v2",
		},
		{
			name:        "invalid URL with space in host returns error",
			input:       "not a url",
			expectError: true,
		},
	}

	for _, tc := range tests {
		suite.Run(tc.name, func() {
			viper.Reset()

			err := SetURLToConfig(tc.input)

			if tc.expectError {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				suite.Equal(tc.expectedURL, viper.GetString(DataRobotURL))
			}
		})
	}
}

func (suite *APITestSuite) TestSetURLToConfigDoesNotWriteFile() {
	err := SetURLToConfig("https://app.datarobot.com")
	suite.Require().NoError(err)

	configFile := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.NoFileExists(configFile, "SetURLToConfig must not write the config file to disk")
}

func (suite *APITestSuite) TestCommandPathToTrace() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "datarobot.cli",
		},
		{
			name:     "root command only",
			input:    "dr",
			expected: "datarobot.cli",
		},
		{
			name:     "single subcommand",
			input:    "dr start",
			expected: "datarobot.cli.start",
		},
		{
			name:     "nested subcommand",
			input:    "dr templates setup",
			expected: "datarobot.cli.templates.setup",
		},
		{
			name:     "deeply nested subcommand",
			input:    "dr self plugin add",
			expected: "datarobot.cli.self.plugin.add",
		},
	}

	for _, tc := range tests {
		suite.Run(tc.name, func() {
			suite.Equal(tc.expected, CommandPathToTrace(tc.input))
		})
	}
}

func (suite *APITestSuite) TestGetSetAPIConsumerTrace() {
	SetAPIConsumerTrace("")

	suite.Equal("datarobot.cli", GetAPIConsumerTrace(), "should fall back to datarobot.cli when unset")

	SetAPIConsumerTrace("datarobot.cli.templates.list")
	suite.Equal("datarobot.cli.templates.list", GetAPIConsumerTrace())

	SetAPIConsumerTrace("datarobot.cli.start")
	suite.Equal("datarobot.cli.start", GetAPIConsumerTrace())
}

func (suite *APITestSuite) TestIsAPIConsumerTrackingEnabled() {
	suite.False(IsAPIConsumerTrackingEnabled(), "should be false when viper has no config set")

	viper.Set(APIConsumerTrackingEnabled, true)
	suite.True(IsAPIConsumerTrackingEnabled())

	viper.Set(APIConsumerTrackingEnabled, false)
	suite.False(IsAPIConsumerTrackingEnabled())
}

// TestCommandPathToTraceWithAliases verifies that cobra always resolves
// command aliases to their canonical Use name before CommandPath() is called.
// This means CommandPathToTrace never receives an alias string — cobra
// normalises it first — so the trace always uses the canonical command name.
func (suite *APITestSuite) TestCommandPathToTraceWithAliases() {
	// Build a small cobra command tree that mirrors real CLI aliases:
	//   root (Use: "dr")
	//   └─ run (Use: "run", Aliases: ["r"])      → like dr run / dr r
	//   └─ start (Use: "start", Aliases: ["quickstart"]) → like dr start / dr quickstart
	//   └─ templates (Use: "templates", Aliases: ["template"])
	//      └─ list (Use: "list")
	root := &cobra.Command{Use: "dr"}

	run := &cobra.Command{Use: "run [tasks]", Aliases: []string{"r"}}
	start := &cobra.Command{Use: "start", Aliases: []string{"quickstart"}}
	templates := &cobra.Command{Use: "templates", Aliases: []string{"template"}}
	list := &cobra.Command{Use: "list"}

	templates.AddCommand(list)
	root.AddCommand(run, start, templates)

	tests := []struct {
		name          string
		args          []string
		expectedTrace string
	}{
		// Canonical invocations
		{"canonical: dr run", []string{"run"}, "datarobot.cli.run"},
		{"canonical: dr start", []string{"start"}, "datarobot.cli.start"},
		{"canonical: dr templates list", []string{"templates", "list"}, "datarobot.cli.templates.list"},
		// Cobra command aliases — CommandPath() must return the canonical name
		{"alias: dr r → dr run", []string{"r"}, "datarobot.cli.run"},
		{"alias: dr quickstart → dr start", []string{"quickstart"}, "datarobot.cli.start"},
		{"alias: dr template list → dr templates list", []string{"template", "list"}, "datarobot.cli.templates.list"},
	}

	for _, tc := range tests {
		suite.Run(tc.name, func() {
			cmd, _, err := root.Find(tc.args)
			suite.Require().NoError(err)
			suite.Require().NotNil(cmd)

			trace := CommandPathToTrace(cmd.CommandPath())
			suite.Equal(tc.expectedTrace, trace)
		})
	}
}

// RedactSecretFields is defence in depth since RedactedReqInfo stopped dumping
// bodies (see TestRedactedReqInfo_OmitsRequestBody). It is still exercised
// directly because it is exported and applied to the header dump.
func TestRedactSecretFields(t *testing.T) {
	const body = `{"name":"my-app/OPENAI_API_KEY","credentialType":"api_token","apiToken":"sk-live-abc123"}`

	out := RedactSecretFields(body)

	assert.NotContains(t, out, "sk-live-abc123")
	assert.Contains(t, out, "[REDACTED]")
	assert.Contains(t, out, "my-app/OPENAI_API_KEY", "the name is not a secret and is what makes a log useful")
	assert.Contains(t, out, "api_token", "the type is not a secret either")
}

func TestRedactSecretFields_CoversTheOtherNames(t *testing.T) {
	for _, field := range []string{"password", "secret", "privateKey", "clientSecret", "refreshToken", "token"} {
		out := RedactSecretFields(`{"` + field + `":"hunter2"}`)
		assert.NotContains(t, out, "hunter2", "field %q", field)
	}
}

// TestRedactedReqInfo_OmitsRequestBody is the regression guard for the pre-GA
// security review finding: `dr pipeline input create --debug` must not write
// the user's payload to the debug log.
//
// The body is arbitrary user-authored JSON -- the natural home for data-source
// credentials -- so a name-keyed redactor cannot be trusted to catch every
// secret in it (api_key, credentials, db_password are all plausible and none
// are in secretBodyFields). The guarantee is therefore that the body is not
// logged at all.
func TestRedactedReqInfo_OmitsRequestBody(t *testing.T) {
	const payload = `{"api_key":"sk-live-NOTREDACTED","db_password":"hunter2","region":"north"}`

	req, err := http.NewRequest(
		http.MethodPost,
		"https://app.datarobot.com/api/v2/pipelines/p-1/inputs",
		strings.NewReader(payload),
	)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-live-TOKEN")

	out := RedactedReqInfo(req)

	// None of the body reaches the log -- not the secrets, and not even the
	// non-secret field, because the body is omitted wholesale rather than
	// filtered.
	assert.NotContains(t, out, "sk-live-NOTREDACTED", "an unlisted secret key must not reach the log")
	assert.NotContains(t, out, "hunter2", "an unlisted secret key must not reach the log")
	assert.NotContains(t, out, "region", "the body is omitted entirely, not filtered")

	// The Authorization header is masked, not dropped.
	assert.NotContains(t, out, "sk-live-TOKEN")
	assert.Contains(t, out, "[REDACTED]")

	// What a debug log is actually for is still there.
	assert.Contains(t, out, "POST")
	assert.Contains(t, out, "/api/v2/pipelines/p-1/inputs")
	assert.Contains(t, out, "Content-Type")
}

// TestRedactedReqInfo_DoesNotDrainBody pins the second half of the fix: a
// headers-only dump leaves req.Body readable, which is what let the
// restoreRequestBody re-arming workaround be deleted from drapi.
func TestRedactedReqInfo_DoesNotDrainBody(t *testing.T) {
	// Deliberately not JSON: this asserts byte-for-byte survival of the body,
	// not JSON equivalence, and a JSON literal makes testifylint push toward
	// assert.JSONEq, which would weaken exactly the property under test.
	const payload = "region=north&limit=10"

	req, err := http.NewRequest(http.MethodPost, "https://example.com/x", strings.NewReader(payload))
	require.NoError(t, err)

	_ = RedactedReqInfo(req)

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body), "the request body must survive being logged")
}
