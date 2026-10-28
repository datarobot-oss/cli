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

package show

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFixtureConfig writes raw as the test's drconfig.yaml under a fresh
// isolated HOME, points the config layer at it, and returns the file's
// absolute path so tests can assert on the path shown in human output.
func writeFixtureConfig(t *testing.T, raw string) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configPath := filepath.Join(configDir, "drconfig.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))

	return configPath
}

func writeTestConfig(t *testing.T) {
	writeFixtureConfig(t, `endpoint: https://app.datarobot.com/api/v2
token: default-token
ca-cert: /etc/ssl/corp.pem
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
  onprem:
    endpoint: https://onprem.example.com/api/v2
    token: onprem-token
    ca-cert: /etc/ssl/onprem.pem
  staging:
    endpoint: https://staging.example.com/api/v2
`)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	t.Cleanup(func() { os.Stdout = orig })

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	fn()

	os.Stdout = orig

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(out)
}

func runShowJSON(t *testing.T, args ...string) (profileDetail, string) {
	t.Helper()

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs(append([]string{"show", "--output-format", "json"}, args...))

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	var envelope struct {
		Profile profileDetail `json:"profile"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))

	return envelope.Profile, out
}

// runShowHuman runs the show command in its default (human) output mode and
// returns its stdout, so tests can assert on human-only rendering such as
// the config path, inheritance wording, and pairing explanations.
func runShowHuman(t *testing.T, args ...string) string {
	t.Helper()

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs(append([]string{"show"}, args...))

	return captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})
}

func TestShow_NamedProfile_OwnCACertNotInherited(t *testing.T) {
	writeTestConfig(t)

	detail, out := runShowJSON(t, "onprem")

	assert.Equal(t, "onprem", detail.Name)
	assert.Equal(t, "https://onprem.example.com/api/v2", detail.Endpoint)
	assert.True(t, detail.EndpointOwn)
	assert.True(t, detail.HasToken)
	assert.Equal(t, "/etc/ssl/onprem.pem", detail.CACert)
	assert.True(t, detail.CACertOwn)
	assert.NotContains(t, out, "onprem-token", "the token value itself must never be printed")
}

func TestShow_NamedProfile_InheritsCACertFromDefault(t *testing.T) {
	writeTestConfig(t)

	detail, _ := runShowJSON(t, "eu-mtsaas")

	assert.Equal(t, "eu-mtsaas", detail.Name)
	assert.Equal(t, "/etc/ssl/corp.pem", detail.CACert, "ca-cert should fall back to the default profile's value")
	assert.False(t, detail.CACertOwn)
}

// TestShow_EndpointOnlyProfileDoesNotInheritCredentials mirrors
// config.applyProfile, which merges endpoint and token atomically: a profile
// created by `auth set-url` alone owns its endpoint and an empty token, and
// must never be shown pairing its own endpoint with the default's token (or
// the default's endpoint under its own name). The *_own flags report
// ownership of the atomic pair, so a profile owning the pair through either
// key reports both flags true.
func TestShow_EndpointOnlyProfileDoesNotInheritCredentials(t *testing.T) {
	writeTestConfig(t)

	detail, _ := runShowJSON(t, "staging")

	assert.Equal(t, "https://staging.example.com/api/v2", detail.Endpoint)
	assert.True(t, detail.EndpointOwn)
	assert.False(t, detail.HasToken, "a profile that owns an endpoint but no token must not borrow the default's")
	assert.True(t, detail.HasTokenOwn)
}

// writeTestConfigWithPairingFixtures builds a config whose root credentials
// are complete (endpoint plus token, no ca-cert) and whose named profiles
// cover every ownership shape the atomic endpoint/token merge can produce:
// endpoint-only, full, token-only, and CA-only (which owns no credential and
// inherits the root pair).
func writeTestConfigWithPairingFixtures(t *testing.T) {
	writeFixtureConfig(t, `endpoint: https://root.example.com/api/v2
token: root-secret-token
profiles:
  caonly:
    ca-cert: /etc/ssl/caonly.pem
  eponly:
    endpoint: https://eponly.example.com/api/v2
  full:
    endpoint: https://full.example.com/api/v2
    token: full-secret-token
  toonly:
    token: toonly-secret-token
`)
}

// TestShow_JSONOwnFlagsArePairAtomic covers the JSON ownership contract for
// every credential ownership shape. endpoint_own and has_token_own report
// ownership of the atomic endpoint/token pair, not per-key presence: both are
// true whenever the profile's own section defines an endpoint, a token, or
// both, and both are false when it defines neither (empty or CA-only), in
// which case the profile inherits the root pair. Only the resolved fields
// (endpoint, has_token) reflect which individual values are present: an
// endpoint-only profile resolves its own endpoint and an empty token, never
// the root token, and a token-only profile resolves no endpoint at all.
func TestShow_JSONOwnFlagsArePairAtomic(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	eponly, _ := runShowJSON(t, "eponly")

	assert.Equal(t, "https://eponly.example.com/api/v2", eponly.Endpoint)
	assert.True(t, eponly.EndpointOwn)
	assert.False(t, eponly.HasToken, "an endpoint-only profile must not inherit the root token")
	assert.True(t, eponly.HasTokenOwn, "owning the pair through the endpoint reports both flags")

	full, _ := runShowJSON(t, "full")

	assert.True(t, full.EndpointOwn)
	assert.True(t, full.HasTokenOwn)
	assert.True(t, full.HasToken)

	toonly, _ := runShowJSON(t, "toonly")

	assert.True(t, toonly.EndpointOwn, "owning the pair through the token reports both flags")
	assert.Empty(t, toonly.Endpoint, "a token-only profile must not inherit the root endpoint")
	assert.True(t, toonly.HasToken)
	assert.True(t, toonly.HasTokenOwn)

	// A CA-only profile owns neither credential: both flags are false and the
	// root endpoint/token pair is inherited whole.
	caonly, _ := runShowJSON(t, "caonly")

	assert.Equal(t, "https://root.example.com/api/v2", caonly.Endpoint, "a profile owning no credential inherits the root pair")
	assert.True(t, caonly.HasToken)
	assert.False(t, caonly.EndpointOwn)
	assert.False(t, caonly.HasTokenOwn)
}

// assertCACertStringKey re-parses a show JSON document and asserts the
// ca_cert key is present with a string value (never omitted or null).
func assertCACertStringKey(t *testing.T, out string) {
	t.Helper()

	var envelope map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))

	profile, ok := envelope["profile"].(map[string]any)
	require.True(t, ok, "show JSON must nest the detail under \"profile\"")

	caCert, ok := profile["ca_cert"]
	require.True(t, ok, "ca_cert must always be present in show JSON")

	assert.IsType(t, "", caCert, "ca_cert must be a JSON string, never null")
}

// TestShow_JSONCACertAlwaysString covers the show-JSON compatibility change:
// ca_cert is emitted even when unset (as the empty string), keeps its value
// when configured, and reports ownership through the unchanged ca_cert_own.
func TestShow_JSONCACertAlwaysString(t *testing.T) {
	writeTestConfig(t)

	// A named profile with no ca-cert of its own inherits the root value.
	detail, out := runShowJSON(t, "eu-mtsaas")

	assert.Equal(t, "/etc/ssl/corp.pem", detail.CACert)
	assert.False(t, detail.CACertOwn)
	assertCACertStringKey(t, out)

	// A named profile with its own ca-cert reports it as owned.
	detail, out = runShowJSON(t, "onprem")

	assert.Equal(t, "/etc/ssl/onprem.pem", detail.CACert)
	assert.True(t, detail.CACertOwn)
	assertCACertStringKey(t, out)

	// The root selection also always carries ca_cert as a string.
	_, out = runShowJSON(t)
	assertCACertStringKey(t, out)
}

// TestShow_JSONCACertEmptyStringWhenUnset covers the unset case on a config
// with no ca-cert anywhere: the key must still be present with the empty
// string, and the root profile's ca_cert_own stays true (the root profile is
// its own fallback, an existing ownership contract this change preserves).
func TestShow_JSONCACertEmptyStringWhenUnset(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	detail, out := runShowJSON(t)

	assert.Empty(t, detail.CACert)
	assert.True(t, detail.CACertOwn, "root ca_cert_own stays true even when unset")
	assertCACertStringKey(t, out)
}

// TestShow_HumanOutputShowsConfigPath covers the human-show config-path line:
// both a named profile and the root selection name the config file actually
// read, so the user can tell which drconfig.yaml the inspection came from.
func TestShow_HumanOutputShowsConfigPath(t *testing.T) {
	configPath := writeFixtureConfig(t, `endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  work:
    endpoint: https://work.example.com/api/v2
`)

	for _, args := range [][]string{nil, {"work"}} {
		out := runShowHuman(t, args...)

		assert.Contains(t, out, configPath, "human show must name the config file actually read (args %v)", args)
	}
}

// TestShow_HumanOutputUnambiguousInheritanceWording covers the root-source
// wording fix: an inherited value is labeled as coming from the root-level
// credentials, not "from default", because the displayed profile may itself
// be called default (the explicit profiles.default section).
func TestShow_HumanOutputUnambiguousInheritanceWording(t *testing.T) {
	writeFixtureConfig(t, `endpoint: https://app.datarobot.com/api/v2
token: default-token
ca-cert: /etc/ssl/corp.pem
profiles:
  work:
    endpoint: https://work.example.com/api/v2
`)

	out := runShowHuman(t, "work")

	assert.Contains(t, out, "inherited from the root-level credentials")
	assert.NotContains(t, out, "inherited from default")
}

// TestShow_HumanEndpointOnlyExplainsUnpairing covers the endpoint-only
// explanation: the profile has its own endpoint, no token of its own, and
// does not use the root-level token, because endpoint and token are paired.
// Nothing is inherited here, so no inheritance suffix may appear either.
func TestShow_HumanEndpointOnlyExplainsUnpairing(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	out := runShowHuman(t, "eponly")

	assert.Contains(t, out, "does not use the root-level token")
	assert.Contains(t, out, "endpoint and token are always paired")

	// The endpoint-only profile owns the credential pair, so its token is
	// simply absent: the token line must not claim root inheritance.
	assert.NotContains(t, out, "not set (inherited", "an endpoint-only profile's token is absent, not inherited")
}

// TestShow_HumanTokenOnlyExplainsUnpairing covers the token-only explanation:
// the profile has its own token but no endpoint of its own and does not
// inherit the root-level endpoint.
func TestShow_HumanTokenOnlyExplainsUnpairing(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	out := runShowHuman(t, "toonly")

	assert.Contains(t, out, "does not inherit the root-level endpoint")
	assert.Contains(t, out, "endpoint and token are always paired")
}

// TestShow_HumanFullProfileHasNoUnpairingExplanation covers the paired case:
// a profile that owns both credentials needs no pairing explanation.
func TestShow_HumanFullProfileHasNoUnpairingExplanation(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	out := runShowHuman(t, "full")

	assert.NotContains(t, out, "root-level token")
	assert.NotContains(t, out, "root-level endpoint")
}

// TestShow_HumanCAOnlyProfileInheritsRootPair covers the human side of the
// empty/CA-only shape: a profile owning neither credential inherits the whole
// root pair (labeled as inherited) and needs no unpairing explanation.
func TestShow_HumanCAOnlyProfileInheritsRootPair(t *testing.T) {
	writeTestConfigWithPairingFixtures(t)

	out := runShowHuman(t, "caonly")

	assert.Contains(t, out, "https://root.example.com/api/v2")
	assert.Contains(t, out, "set (inherited from the root-level credentials)")
	assert.NotContains(t, out, "endpoint and token are always paired")
}

// TestShow_HumanRootTitleIsLabeled covers the distinct root labeling: the
// root-level selection is displayed as "default (root-level)" so it cannot
// be confused with an explicit profiles.default section.
func TestShow_HumanRootTitleIsLabeled(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	rootOut := runShowHuman(t)
	assert.Contains(t, rootOut, "default (root-level)")

	explicitOut := runShowHuman(t, defaultProfileLabel)

	// Compare the title line specifically: later lines may legitimately name
	// the root-level credentials (e.g. an inherited ca-cert suffix).
	title := strings.SplitN(explicitOut, "\n", 2)[0]

	assert.Equal(t, "default (explicit)", title)
	assert.NotContains(t, strings.TrimPrefix(explicitOut, title), "(root-level)", "the explicit default must not be labeled as root")
}

func TestShow_NoArgsDefaultsToActiveProfile(t *testing.T) {
	writeTestConfig(t)

	viperx.Set(config.ProfileKey, "eu-mtsaas")
	require.NoError(t, config.ReadConfigFile(""))

	detail, _ := runShowJSON(t)

	assert.Equal(t, "eu-mtsaas", detail.Name)
	assert.True(t, detail.Active)
}

func TestShow_ExplicitDefaultLabel(t *testing.T) {
	writeTestConfig(t)

	detail, _ := runShowJSON(t, defaultProfileLabel)

	assert.Equal(t, defaultProfileLabel, detail.Name)
	assert.Equal(t, "https://app.datarobot.com/api/v2", detail.Endpoint)
	assert.True(t, detail.Active)
}

// writeTestConfigWithExplicitDefault adds an explicit profiles.default
// section, distinct from the root-level credentials, to the shared fixture.
func writeTestConfigWithExplicitDefault(t *testing.T) {
	writeFixtureConfig(t, `endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  default:
    endpoint: https://explicit.example.com/api/v2
    token: explicit-token
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)
}

// TestShow_PositionalDefaultPrefersExplicitSection covers the positional
// compatibility rule: `auth profile show default` shows the explicit
// profiles.default section when one is defined, not the root credentials.
func TestShow_PositionalDefaultPrefersExplicitSection(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	detail, out := runShowJSON(t, defaultProfileLabel)

	assert.Equal(t, defaultProfileLabel, detail.Name)
	assert.Equal(t, "https://explicit.example.com/api/v2", detail.Endpoint, "positional show default must prefer the explicit section")
	assert.True(t, detail.HasTokenOwn)
	assert.NotContains(t, out, "explicit-token", "the token value itself must never be printed")
}

// TestShow_PositionalDefaultNormalizesSpellings covers normalized positional
// spellings: " DEFAULT " resolves to the same explicit section as "default".
func TestShow_PositionalDefaultNormalizesSpellings(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	detail, _ := runShowJSON(t, "  DEFAULT  ")

	assert.Equal(t, defaultProfileLabel, detail.Name)
	assert.Equal(t, "https://explicit.example.com/api/v2", detail.Endpoint)
}

// TestShow_FlagDefaultSelectsExplicitSection covers --profile default /
// DATAROBOT_CLI_PROFILE=default resolving to the explicit section, marked
// active, with the root credentials never shown in its place.
func TestShow_FlagDefaultSelectsExplicitSection(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	viperx.Set(config.ProfileKey, "default")
	require.NoError(t, config.ReadConfigFile(""))

	detail, _ := runShowJSON(t)

	assert.Equal(t, defaultProfileLabel, detail.Name)
	assert.Equal(t, "https://explicit.example.com/api/v2", detail.Endpoint)
	assert.True(t, detail.Active)
}

// TestShow_OmittedSelectionStaysRoot covers omission semantics with an
// explicit profiles.default present: no selection resolves to the
// root-level credentials, never to profiles.default.
func TestShow_OmittedSelectionStaysRoot(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	detail, _ := runShowJSON(t)

	assert.Equal(t, defaultProfileLabel, detail.Name, "the root profile is displayed as %q", defaultProfileLabel)
	assert.Equal(t, "https://app.datarobot.com/api/v2", detail.Endpoint, "omitted selection must resolve to root, not profiles.default")
	assert.True(t, detail.Active)
}

func TestShow_UnknownProfileErrors(t *testing.T) {
	writeTestConfig(t)

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs([]string{"show", "does-not-exist"})
	root.SilenceErrors = true
	root.SilenceUsage = true

	err := root.Execute()
	require.Error(t, err)

	// The positional path must render the same actionable message as the
	// flag/env selection path: normalized name, explicit candidates, and
	// list/create remediation, with no wrapper noise.
	expected := (&config.UnknownProfileError{
		Name:  "does-not-exist",
		Known: []string{"eu-mtsaas", "onprem", "staging"},
	}).Error()

	assert.Equal(t, expected, err.Error())
	assert.Contains(t, err.Error(), "does-not-exist")
	assert.Contains(t, err.Error(), "eu-mtsaas, onprem, staging")
	assert.Contains(t, err.Error(), "dr auth profile list")
	assert.Contains(t, err.Error(), "dr auth login --profile does-not-exist")
}
