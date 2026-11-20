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

package list

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestConfig(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	raw := `endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
  no-token:
    endpoint: https://notoken.example.com/api/v2
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drconfig.yaml"), []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))
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

// captureBothStreams swaps os.Stdout and os.Stderr for pipes while fn runs,
// returning what was written to each. Guidance such as the profile-creation
// hint goes to stderr while the table goes to stdout, so a single run can
// legitimately write to both.
func captureBothStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	origOut, origErr := os.Stdout, os.Stderr

	t.Cleanup(func() {
		os.Stdout = origOut
		os.Stderr = origErr
	})

	outR, outW, err := os.Pipe()
	require.NoError(t, err)

	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = outW
	os.Stderr = errW

	fn()

	os.Stdout = origOut
	os.Stderr = origErr

	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())

	outBytes, err := io.ReadAll(outR)
	require.NoError(t, err)

	errBytes, err := io.ReadAll(errR)
	require.NoError(t, err)

	return string(outBytes), string(errBytes)
}

// runListStreams runs `list` with the given arguments (in the default human
// output mode unless the arguments set --output-format themselves) and
// returns stdout and stderr separately.
func runListStreams(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs(append([]string{"list"}, args...))

	return captureBothStreams(t, func() {
		require.NoError(t, root.Execute())
	})
}

func TestList_JSON(t *testing.T) {
	writeTestConfig(t)

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs([]string{"list", "--output-format", "json"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	var envelope struct {
		Profiles []profileOutput `json:"profiles"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	require.Len(t, envelope.Profiles, 3)

	byName := map[string]profileOutput{}
	for _, p := range envelope.Profiles {
		byName[p.Name] = p
	}

	def := byName[defaultProfileLabel]
	assert.Equal(t, "https://app.datarobot.com/api/v2", def.Endpoint)
	assert.True(t, def.HasToken)
	assert.True(t, def.Active, "the default profile is active when no --profile is given")

	eu := byName["eu-mtsaas"]
	assert.Equal(t, "https://app.eu.datarobot.com/api/v2", eu.Endpoint)
	assert.True(t, eu.HasToken)
	assert.False(t, eu.Active)

	noToken := byName["no-token"]
	assert.False(t, noToken.HasToken)

	assert.NotContains(t, out, "default-token", "the token value itself must never be printed")
	assert.NotContains(t, out, "eu-token")
}

func TestList_Text(t *testing.T) {
	writeTestConfig(t)

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs([]string{"list"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	assert.Contains(t, out, defaultProfileLabel)
	assert.Contains(t, out, "eu-mtsaas")
	assert.Contains(t, out, "no-token")
	assert.NotContains(t, out, "default-token")
	assert.NotContains(t, out, "eu-token")
}

func TestList_ActiveProfileMarked(t *testing.T) {
	writeTestConfig(t)

	viperx.Set(config.ProfileKey, "eu-mtsaas")

	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs([]string{"list", "--output-format", "json"})

	out := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	var envelope struct {
		Profiles []profileOutput `json:"profiles"`
	}

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))

	for _, p := range envelope.Profiles {
		assert.Equal(t, p.Name == "eu-mtsaas", p.Active, "profile %q active flag", p.Name)
	}
}

// TestList_ExplicitDefaultDistinctFromRoot covers a config with an explicit
// profiles.default section: root and the explicit section are distinct rows,
// and both are labeled so neither can be confused with the other.
func TestList_ExplicitDefaultDistinctFromRoot(t *testing.T) {
	writeTestConfigWithExplicitDefault(t)

	// Human output: two visibly distinct rows.
	root := &cobra.Command{Use: "test"}

	var format outputformat.OutputFormat

	outputformat.AddPersistentFlag(root, &format)
	root.AddCommand(Cmd())
	root.SetArgs([]string{"list"})

	humanOut := captureStdout(t, func() {
		require.NoError(t, root.Execute())
	})

	assert.Contains(t, humanOut, "default (explicit)", "the explicit profiles.default row must be labeled apart from root")
	assert.Contains(t, humanOut, "default (root-level)", "the root row must be labeled apart from the explicit default")

	// JSON output: both entries appear with their stored names.
	rootJSON := &cobra.Command{Use: "test"}

	outputformat.AddPersistentFlag(rootJSON, &format)
	rootJSON.AddCommand(Cmd())
	rootJSON.SetArgs([]string{"list", "--output-format", "json"})

	jsonOut := captureStdout(t, func() {
		require.NoError(t, rootJSON.Execute())
	})

	var envelope struct {
		Profiles []profileOutput `json:"profiles"`
	}

	require.NoError(t, json.Unmarshal([]byte(jsonOut), &envelope))

	var explicitDefaults []profileOutput

	for _, p := range envelope.Profiles {
		if p.Name == defaultProfileLabel {
			explicitDefaults = append(explicitDefaults, p)
		}
	}

	assert.Len(t, explicitDefaults, 2, "root and the explicit profiles.default section are distinct rows")

	// No selection marks root, not the explicit section, active.
	activeCount := 0

	for _, p := range envelope.Profiles {
		if p.Active {
			activeCount++
		}
	}

	assert.Equal(t, 1, activeCount, "exactly one row is active with no selection, and it is root")
}

// writeTestConfigWithExplicitDefault adds root credentials plus an explicit
// profiles.default section and one sibling profile.
func writeTestConfigWithExplicitDefault(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	raw := `endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  default:
    endpoint: https://explicit.example.com/api/v2
    token: explicit-token
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drconfig.yaml"), []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))
}

// writeTestConfigExplicitDefaultOnly builds a config with an explicit
// profiles.default section but no root-level credentials, so list must not
// mark the empty root row as holding the active credentials.
func writeTestConfigExplicitDefaultOnly(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	raw := `profiles:
  default:
    endpoint: https://explicit.example.com/api/v2
    token: explicit-token
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drconfig.yaml"), []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))
}

// TestList_ExplicitDefaultOnlyNotMarkedActiveWithoutSelection covers the
// active-marker rule for a config with no root credentials: with no
// selection, no row may be marked active -- the root row is what omission
// resolves to, but it holds no credentials, and the explicit default is not
// selected either.
func TestList_ExplicitDefaultOnlyNotMarkedActiveWithoutSelection(t *testing.T) {
	writeTestConfigExplicitDefaultOnly(t)

	stdout, _ := runListStreams(t, "--output-format", "json")

	var envelope struct {
		Profiles []profileOutput `json:"profiles"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.Len(t, envelope.Profiles, 2, "root and the explicit default are distinct rows")

	activeCount := 0

	for _, p := range envelope.Profiles {
		assert.False(t, p.Active, "no row may claim the active credentials without a selection (row %q)", p.Name)

		if p.Active {
			activeCount++
		}
	}

	assert.Equal(t, 0, activeCount)

	// Selecting the explicit default marks exactly that row active.
	viperx.Set(config.ProfileKey, "default")
	require.NoError(t, config.ReadConfigFile(""))

	stdout, _ = runListStreams(t, "--output-format", "json")

	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))

	for _, p := range envelope.Profiles {
		assert.Equal(t, p.Name == defaultProfileLabel && p.Endpoint != "", p.Active, "row %q active flag with --profile default", p.Name)
	}
}

// writeTestConfigNoProfiles builds a config with root credentials and no
// profiles key at all, the state the creation hint targets.
func writeTestConfigNoProfiles(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	raw := `endpoint: https://app.datarobot.com/api/v2
token: default-token
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "drconfig.yaml"), []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))
}

// TestList_CreationHintWhenNoExplicitProfiles covers the creation hint: with
// no explicit profiles, human list suggests real create commands on stderr,
// while JSON output stays pure (the hint never touches JSON stdout).
func TestList_CreationHintWhenNoExplicitProfiles(t *testing.T) {
	writeTestConfigNoProfiles(t)

	stdout, stderr := runListStreams(t)

	assert.Contains(t, stdout, defaultProfileLabel, "the root row is still listed")
	assert.Contains(t, stderr, "dr auth login --profile", "the hint must name a real create command")
	assert.Contains(t, stderr, "dr auth set-url --profile", "the hint must name a real create command")

	// JSON mode: stdout parses and carries no hint text; stderr stays clean.
	jsonStdout, jsonStderr := runListStreams(t, "--output-format", "json")

	var envelope struct {
		Profiles []profileOutput `json:"profiles"`
	}

	require.NoError(t, json.Unmarshal([]byte(jsonStdout), &envelope))
	assert.NotContains(t, jsonStdout, "dr auth login --profile", "JSON stdout must stay machine-readable")
	assert.Empty(t, jsonStderr, "the hint is a human-mode convenience only")
}

// TestList_CreationHintForEmptyConfig covers the config-absent/empty state:
// list exits successfully and shows the same kind of creation hint.
func TestList_CreationHintForEmptyConfig(t *testing.T) {
	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	require.NoError(t, config.ReadConfigFile(""))

	stdout, stderr := runListStreams(t)

	assert.Contains(t, stderr, "dr auth login --profile")
	assert.Contains(t, stderr, "dr auth set-url --profile")
	assert.NotContains(t, stdout, "login --profile", "the hint must not pollute stdout")
}

// TestList_NoHintWhenExplicitProfilesExist covers the counterpart rule: any
// explicit profile -- including only an explicit default -- suppresses the
// creation hint.
func TestList_NoHintWhenExplicitProfilesExist(t *testing.T) {
	writeTestConfig(t)

	_, stderr := runListStreams(t)
	assert.NotContains(t, stderr, "dr auth login --profile")

	writeTestConfigExplicitDefaultOnly(t)

	_, stderr = runListStreams(t)
	assert.NotContains(t, stderr, "dr auth login --profile")
}
