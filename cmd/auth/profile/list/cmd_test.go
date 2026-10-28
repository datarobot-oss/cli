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
// and the explicit row is labeled so it cannot be confused with root.
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
