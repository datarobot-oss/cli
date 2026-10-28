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

package del

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeTestConfig writes the delete fixture used by the command tests and
// points viper at it, mirroring the show command's test seam.
func writeTestConfig(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := `# header comment
endpoint: https://app.datarobot.com/api/v2
token: root-token

profiles:
  # alpha keeps its own comment
  alpha:
    endpoint: https://alpha.example.com/api/v2
    token: alpha-token
  beta:
    endpoint: https://beta.example.com/api/v2
  gamma:
    endpoint: https://gamma.example.com/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))
	require.NoError(t, config.ReadConfigFile(""))

	return configFile
}

func readConfigBytes(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func readConfigProfiles(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var m map[string]any

	require.NoError(t, yaml.Unmarshal(data, &m))

	profiles, _ := m["profiles"].(map[string]any)

	return profiles
}

// runVerb executes the delete command (via the given verb: "delete" or its
// "rm" alias) in a minimal cobra tree, capturing stdout, and returns the
// captured output plus the command error.
func runVerb(t *testing.T, verb string, args ...string) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "test"}
	root.AddCommand(Cmd())
	root.SetArgs(append([]string{verb}, args...))

	var out strings.Builder

	orig := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	cmdErr := root.Execute()

	os.Stdout = orig

	require.NoError(t, w.Close())

	_, err = io.Copy(&out, r)
	require.NoError(t, err)

	return out.String(), cmdErr
}

// runDeleteCmd executes `delete` with the given arguments.
func runDeleteCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	return runVerb(t, "delete", args...)
}

func TestDelete_RemovesProfileAndConfirmsName(t *testing.T) {
	configFile := writeTestConfig(t)

	out, cmdErr := runDeleteCmd(t, "beta")
	require.NoError(t, cmdErr)

	assert.Contains(t, out, "beta", "the confirmation must name the deleted profile")
	assert.NotContains(t, out, "alpha", "unrelated profiles are not mentioned")
	assert.NotContains(t, out, "root-token", "no token value may ever be printed")

	profiles := readConfigProfiles(t, configFile)
	assert.NotContains(t, profiles, "beta")
	assert.Contains(t, profiles, "alpha")
	assert.Contains(t, profiles, "gamma")

	assert.Contains(t, readConfigBytes(t, configFile), "# header comment")
}

func TestDelete_RmAliasBehavesLikeDelete(t *testing.T) {
	// Each run gets a fresh fixture: both spellings must succeed against an
	// intact config and remove exactly the named section.
	configFile := writeTestConfig(t)

	outDelete, errDelete := runDeleteCmd(t, "alpha")
	require.NoError(t, errDelete)
	assert.Contains(t, outDelete, "alpha")

	profiles := readConfigProfiles(t, configFile)
	assert.NotContains(t, profiles, "alpha")

	configFile = writeTestConfig(t)

	outRm, errRm := runVerb(t, "rm", "alpha")
	require.NoError(t, errRm)
	assert.Contains(t, outRm, "alpha")

	profiles = readConfigProfiles(t, configFile)
	assert.NotContains(t, profiles, "alpha")
	assert.Contains(t, profiles, "beta")
	assert.Contains(t, profiles, "gamma")
}

func TestDelete_NormalizesNameAndConfirmsNormalized(t *testing.T) {
	configFile := writeTestConfig(t)

	out, cmdErr := runDeleteCmd(t, " BETA ")
	require.NoError(t, cmdErr)

	assert.Contains(t, out, `"beta"`, "the confirmation names the normalized profile")

	profiles := readConfigProfiles(t, configFile)
	assert.NotContains(t, profiles, "beta")
}

func TestDelete_RefusesDefaultWithLogoutGuidance(t *testing.T) {
	configFile := writeTestConfig(t)

	before := readConfigBytes(t, configFile)

	for _, name := range []string{"default", "DEFAULT", " default "} {
		_, cmdErr := runDeleteCmd(t, name)
		require.Error(t, cmdErr, "deleting %q must be refused", name)
		assert.Contains(t, cmdErr.Error(), "dr auth logout", "the refusal must point at logout")
		assert.NotContains(t, cmdErr.Error(), "alpha-token")
	}

	assert.Equal(t, before, readConfigBytes(t, configFile), "a refused deletion must not touch the file")
}

func TestDelete_MissingProfileListsKnownNames(t *testing.T) {
	configFile := writeTestConfig(t)

	before := readConfigBytes(t, configFile)

	_, cmdErr := runDeleteCmd(t, "nope")
	require.Error(t, cmdErr)

	assert.Contains(t, cmdErr.Error(), `"nope"`)
	assert.Contains(t, cmdErr.Error(), "alpha, beta, gamma")
	assert.Contains(t, cmdErr.Error(), "dr auth profile list")
	assert.NotContains(t, cmdErr.Error(), "alpha-token")

	assert.Equal(t, before, readConfigBytes(t, configFile))
}

func TestDelete_MissingProfileTakesUnknownProfileError(t *testing.T) {
	writeTestConfig(t)

	_, cmdErr := runDeleteCmd(t, "nope")
	require.Error(t, cmdErr)

	var unknown *config.UnknownProfileError
	assert.ErrorAs(t, cmdErr, &unknown, "the RunE error must stay typed for callers")
}

func TestDelete_RequiresExactlyOneArgument(t *testing.T) {
	writeTestConfig(t)

	for _, args := range [][]string{{}, {"alpha", "beta"}} {
		_, cmdErr := runDeleteCmd(t, args...)
		require.Error(t, cmdErr, "argv %v must be rejected as a usage error", args)
	}
}
