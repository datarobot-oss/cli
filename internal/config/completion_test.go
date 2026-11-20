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
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCompletionConfig writes raw YAML to a fresh temp file and returns its
// path, so completion readback tests never touch a real drconfig.yaml.
func writeCompletionConfig(t *testing.T, raw string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "drconfig.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	return path
}

// TestProfileNamesFromFile_OffersEveryExplicitSection covers the multi-profile
// fixture: every explicit section is offered with --profile normalization,
// while the root-level credentials (no section) never appear.
func TestProfileNamesFromFile_OffersEveryExplicitSection(t *testing.T) {
	path := writeCompletionConfig(t, `endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
  East:
    endpoint: https://east.invalid/api/v2
    token: dummy-east-token
  west:
    endpoint: https://west.invalid/api/v2
    token: dummy-west-token
`)

	names, err := ProfileNamesFromFile(path)
	require.NoError(t, err)

	assert.Equal(t, []string{"east", "west"}, names)
}

// TestProfileNamesFromFile_IncludesExplicitDefaultOnlyWhenDefined covers the
// explicit-default fixture: "default" is a legal candidate exactly because a
// profiles.default section is defined; it must not be invented.
func TestProfileNamesFromFile_IncludesExplicitDefaultOnlyWhenDefined(t *testing.T) {
	path := writeCompletionConfig(t, `endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
  default:
    endpoint: https://default.invalid/api/v2
    token: dummy-default-token
  work:
    endpoint: https://work.invalid/api/v2
    token: dummy-work-token
`)

	names, err := ProfileNamesFromFile(path)
	require.NoError(t, err)

	assert.Equal(t, []string{"default", "work"}, names)
}

// TestProfileNamesFromFile_RootOnlyYieldsNone covers the root-only fixture: no
// explicit sections means no completion candidates at all.
func TestProfileNamesFromFile_RootOnlyYieldsNone(t *testing.T) {
	path := writeCompletionConfig(t, `endpoint: https://root.invalid/api/v2
token: dummy-root-token
`)

	names, err := ProfileNamesFromFile(path)
	require.NoError(t, err)

	assert.Empty(t, names)
}

// TestProfileNamesFromFile_EmptyProfilesBlockYieldsNone covers a present but
// empty/null profiles mapping: no sections are defined, so nothing is offered.
func TestProfileNamesFromFile_EmptyProfilesBlockYieldsNone(t *testing.T) {
	path := writeCompletionConfig(t, `endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
`)

	names, err := ProfileNamesFromFile(path)
	require.NoError(t, err)

	assert.Empty(t, names)
}

// TestProfileNamesFromFile_MissingFileYieldsNoneWithoutError covers a config
// path that does not exist: completion treats it as "no profiles defined"
// rather than an error, so shells still get a clean (empty) candidate list.
func TestProfileNamesFromFile_MissingFileYieldsNoneWithoutError(t *testing.T) {
	names, err := ProfileNamesFromFile(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)

	assert.Empty(t, names)
}

// TestProfileNamesFromFile_InvalidYAMLIsAnError covers an unparsable file: the
// error is returned so the completion layer can decide how to stay silent.
func TestProfileNamesFromFile_InvalidYAMLIsAnError(t *testing.T) {
	path := writeCompletionConfig(t, "profiles: [unclosed")

	_, err := ProfileNamesFromFile(path)
	assert.Error(t, err)
}

// TestProfileNamesFromFile_CaseDuplicateSectionsDedupe covers the viper
// case-insensitive key model: two sections that differ only by case collapse
// to one normalized candidate, matching how --profile selection resolves.
func TestProfileNamesFromFile_CaseDuplicateSectionsDedupe(t *testing.T) {
	path := writeCompletionConfig(t, `endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
  East:
    endpoint: https://east.invalid/api/v2
    token: dummy-east-token
  east:
    endpoint: https://east2.invalid/api/v2
    token: dummy-east2-token
`)

	names, err := ProfileNamesFromFile(path)
	require.NoError(t, err)

	assert.Equal(t, []string{"east"}, names)
}

// TestCompletionConfigPath_ExplicitPathWins covers the --config flag beating
// DATAROBOT_CLI_CONFIG, mirroring the pre-run's resolution order.
func TestCompletionConfigPath_ExplicitPathWins(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_CONFIG", "/env/selected/drconfig.yaml")

	assert.Equal(t, "/flag/selected/drconfig.yaml", CompletionConfigPath("/flag/selected/drconfig.yaml"))
}

// TestCompletionConfigPath_EnvFallback covers the env var being used when no
// --config value was parsed for the completion invocation.
func TestCompletionConfigPath_EnvFallback(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_CONFIG", "/env/selected/drconfig.yaml")

	assert.Equal(t, "/env/selected/drconfig.yaml", CompletionConfigPath(""))
}

// TestCompletionConfigPath_DefaultLocation covers the no-flag, no-env case:
// the default drconfig.yaml under the (XDG-reloaded) config directory, the
// same location the normal pre-run reads.
func TestCompletionConfigPath_DefaultLocation(t *testing.T) {
	tempHome := t.TempDir()

	testutil.SetTestHomeDir(t, tempHome)

	dir, err := GetConfigDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "drconfig.yaml"), CompletionConfigPath(""))
}
