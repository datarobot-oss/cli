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
ca-cert: /etc/ssl/corp.pem
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
  onprem:
    endpoint: https://onprem.example.com/api/v2
    token: onprem-token
    ca-cert: /etc/ssl/onprem.pem
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
	assert.Contains(t, err.Error(), "does-not-exist")
	assert.Contains(t, err.Error(), "eu-mtsaas")
}
