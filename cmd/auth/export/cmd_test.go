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

package export

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	internalShell "github.com/datarobot/cli/internal/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearEnvCredentials removes the ambient DataRobot environment variables so a
// developer's own shell cannot influence the result.
func clearEnvCredentials(t *testing.T) {
	t.Helper()

	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")
}

func TestCanonicalEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"bare host", "app.datarobot.com", "https://app.datarobot.com/api/v2", false},
		{"scheme only", "https://app.datarobot.com", "https://app.datarobot.com/api/v2", false},
		{"trailing slash", "https://app.datarobot.com/", "https://app.datarobot.com/api/v2", false},
		{"already canonical", "https://app.datarobot.com/api/v2", "https://app.datarobot.com/api/v2", false},
		{"custom api path", "https://dr.corp.example/dr/api/v2", "https://dr.corp.example/dr/api/v2", false},
		{"strips query", "https://app.datarobot.com/api/v2?x=1", "https://app.datarobot.com/api/v2", false},
		{"whitespace", "  https://app.datarobot.com  ", "https://app.datarobot.com/api/v2", false},
		{"port preserved", "https://dr.corp.example:8443", "https://dr.corp.example:8443/api/v2", false},
		{"single-quoted url is rejected", "'https://app.datarobot.com/api/v2'", "", true},
		{"double-quoted url is rejected", `"https://app.datarobot.com/api/v2"`, "", true},
		{"empty", "", "", true},
		{"no host", "https://", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := canonicalEndpoint(c.in)
			if c.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestRenderExports(t *testing.T) {
	creds := credentials{
		Endpoint: "https://app.datarobot.com/api/v2",
		Token:    "NjY2",
	}

	cases := []struct {
		shell internalShell.Shell
		want  []string
	}{
		{
			internalShell.Bash,
			[]string{
				"export DATAROBOT_ENDPOINT='https://app.datarobot.com/api/v2'",
				"export DATAROBOT_API_TOKEN='NjY2'",
			},
		},
		{
			internalShell.Zsh,
			[]string{
				"export DATAROBOT_ENDPOINT='https://app.datarobot.com/api/v2'",
				"export DATAROBOT_API_TOKEN='NjY2'",
			},
		},
		{
			internalShell.Fish,
			[]string{
				"set -gx DATAROBOT_ENDPOINT 'https://app.datarobot.com/api/v2'",
				"set -gx DATAROBOT_API_TOKEN 'NjY2'",
			},
		},
		{
			internalShell.PowerShell,
			[]string{
				"$env:DATAROBOT_ENDPOINT = 'https://app.datarobot.com/api/v2'",
				"$env:DATAROBOT_API_TOKEN = 'NjY2'",
			},
		},
		{
			internalShell.Cmd,
			[]string{
				"set DATAROBOT_ENDPOINT=https://app.datarobot.com/api/v2",
				"set DATAROBOT_API_TOKEN=NjY2",
			},
		},
		{
			internalShell.Shell("dash"),
			[]string{
				"export DATAROBOT_ENDPOINT='https://app.datarobot.com/api/v2'",
				"export DATAROBOT_API_TOKEN='NjY2'",
			},
		},
	}

	for _, c := range cases {
		t.Run(string(c.shell), func(t *testing.T) {
			assert.Equal(t, c.want, renderExports(c.shell, creds))
		})
	}
}

func TestQuotingEscapesEmbeddedQuotes(t *testing.T) {
	nasty := credentials{Endpoint: "https://h/api/v2", Token: "a'b&c"}

	assert.Equal(t, `export DATAROBOT_API_TOKEN='a'\''b&c'`, renderExports(internalShell.Bash, nasty)[1])
	assert.Equal(t, `$env:DATAROBOT_API_TOKEN = 'a''b&c'`, renderExports(internalShell.PowerShell, nasty)[1])
	assert.Equal(t, `set DATAROBOT_API_TOKEN=a'b^&c`, renderExports(internalShell.Cmd, nasty)[1])
}

func TestResolveShell(t *testing.T) {
	t.Run("explicit shell is normalized", func(t *testing.T) {
		got, err := resolveShell("  FISH ")
		require.NoError(t, err)
		assert.Equal(t, internalShell.Fish, got)
	})

	t.Run("unsupported shell errors", func(t *testing.T) {
		_, err := resolveShell("nushell")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nushell")
	})

	t.Run("empty shell is detected", func(t *testing.T) {
		// Detection depends on the host environment; any non-empty result is
		// acceptable as long as it never fails.
		got, err := resolveShell("")
		require.NoError(t, err)
		assert.NotEmpty(t, string(got))
	})
}

func TestResolveCredentials_PrefersCompleteEnvPair(t *testing.T) {
	viperx.Reset()

	t.Setenv("DATAROBOT_ENDPOINT", "env.datarobot.com")
	t.Setenv("DATAROBOT_API_TOKEN", "env-token")

	viperx.Set(config.DataRobotURL, "https://config.datarobot.com/api/v2")
	viperx.Set(config.DataRobotAPIKey, "config-token")

	defer viperx.Reset()

	creds, err := resolveCredentials()
	require.NoError(t, err)
	assert.Equal(t, "https://env.datarobot.com/api/v2", creds.Endpoint)
	assert.Equal(t, "env-token", creds.Token)
}

func TestResolveCredentials_FallsBackToConfig(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	// A half-set environment must not be mixed with the config file.
	t.Setenv("DATAROBOT_ENDPOINT", "env.datarobot.com")

	viperx.Set(config.DataRobotURL, "https://config.datarobot.com/api/v2")
	viperx.Set(config.DataRobotAPIKey, "config-token")

	defer viperx.Reset()

	creds, err := resolveCredentials()
	require.NoError(t, err)
	assert.Equal(t, "https://config.datarobot.com/api/v2", creds.Endpoint)
	assert.Equal(t, "config-token", creds.Token)
}

func TestResolveCredentials_MissingValues(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		token    string
		wantErr  error
	}{
		{"nothing configured", "", "", errNoCredentials},
		{"no endpoint", "", "some-token", errNoEndpoint},
		{"no token", "https://app.datarobot.com/api/v2", "", errNoToken},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			viperx.Reset()
			clearEnvCredentials(t)

			viperx.Set(config.DataRobotURL, c.endpoint)
			viperx.Set(config.DataRobotAPIKey, c.token)

			defer viperx.Reset()

			_, err := resolveCredentials()
			require.ErrorIs(t, err, c.wantErr)
		})
	}
}

func TestRunE_WritesSourceableStatements(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	viperx.Set(config.DataRobotURL, "https://app.datarobot.com/api/v2")
	viperx.Set(config.DataRobotAPIKey, "secret-token")

	defer viperx.Reset()

	var stdout, stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--shell", "bash"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t,
		"export DATAROBOT_ENDPOINT='https://app.datarobot.com/api/v2'\n"+
			"export DATAROBOT_API_TOKEN='secret-token'\n",
		stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunE_JSONOutput(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	viperx.Set(config.DataRobotURL, "app.datarobot.com")
	viperx.Set(config.DataRobotAPIKey, "secret-token")

	defer viperx.Reset()

	var stdout bytes.Buffer

	var format outputformat.OutputFormat

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	// The real --output-format flag is persistent on the root command; add it
	// locally so the subcommand can be exercised standalone.
	outputformat.AddFlag(cmd, &format)
	cmd.SetArgs([]string{"--output-format", "json"})

	require.NoError(t, cmd.Execute())

	var payload struct {
		Environment map[string]string `json:"environment"`
		Source      string            `json:"source"`
	}

	require.NoError(t, json.Unmarshal(stdout.Bytes(), &payload))
	assert.Equal(t, map[string]string{
		"DATAROBOT_ENDPOINT":  "https://app.datarobot.com/api/v2",
		"DATAROBOT_API_TOKEN": "secret-token",
	}, payload.Environment)
	assert.Equal(t, "drconfig.yaml", payload.Source)
}

func TestRunE_QuotedEnvEndpointFailsWithEvalHint(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	// Reproduces the real footgun: `$(dr auth export)` (no eval) bakes the
	// output's single quotes into DATAROBOT_ENDPOINT. The CLI must NOT strip
	// them; it must fail and point the user at `eval "$(dr auth export)"`.
	t.Setenv("DATAROBOT_ENDPOINT", "'https://staging.datarobot.com/api/v2'")
	t.Setenv("DATAROBOT_API_TOKEN", "token")

	defer viperx.Reset()

	var stdout, stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--shell", "bash"})

	err := cmd.Execute()
	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "DATAROBOT_ENDPOINT environment variable")
	assert.NotContains(t, stderr.String(), "drconfig.yaml")
	assert.Contains(t, stderr.String(), `eval "$(dr auth export)"`)
	assert.Contains(t, stderr.String(), "unset DATAROBOT_ENDPOINT")
	assert.NotContains(t, stderr.String(), "auth set-url")
}

func TestRunE_QuotedConfigEndpointNamesSourceWithoutEvalHint(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	// A quoted endpoint coming from the config file (not env) still fails and
	// names drconfig.yaml as the source, but does NOT show the `eval` hint
	// (that hint is specific to the env `$(...)` misuse).
	viperx.Set(config.DataRobotURL, "'https://staging.datarobot.com/api/v2'")
	viperx.Set(config.DataRobotAPIKey, "token")

	defer viperx.Reset()

	var stdout, stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--shell", "bash"})

	err := cmd.Execute()
	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "drconfig.yaml")
	assert.NotContains(t, stderr.String(), `eval "$(dr auth export)"`)
	assert.Contains(t, stderr.String(), "auth set-url")
	assert.NotContains(t, stderr.String(), "unset DATAROBOT_ENDPOINT")
}

func TestRunE_NoCredentialsKeepsStdoutEmpty(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	defer viperx.Reset()

	var stdout, stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.ErrorIs(t, err, cli.ErrSilent)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "auth login")
}

func TestRunE_UnsupportedShell(t *testing.T) {
	viperx.Reset()
	clearEnvCredentials(t)

	defer viperx.Reset()

	var stdout, stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--shell", "nushell"})

	require.Error(t, cmd.Execute())
	assert.Empty(t, stdout.String())
}
