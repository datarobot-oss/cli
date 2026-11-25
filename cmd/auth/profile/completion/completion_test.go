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

// The completion tests run real `dr __complete` probes against a built (but
// isolated) root command tree, so they cover the full wiring: the --profile
// flag completion registered on the root, the positional ValidArgsFunction on
// show/delete/rm, cobra's candidate/directive output protocol, and the
// no-side-effect guarantees (no config mutation).
package completion_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bothFixture mirrors the contract's F-BOTH: root credentials plus explicit
// default, east, endpoint-only, and west sections.
const bothFixture = `# root-level credentials
endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
  # explicit profiles.default section
  default:
    endpoint: https://default.invalid/api/v2
    token: dummy-default-token
  east:
    endpoint: https://east.invalid/api/v2
    token: dummy-east-token
  endpoint-only:
    endpoint: https://endpoint-only.invalid/api/v2
  west:
    endpoint: https://west.invalid/api/v2
    token: dummy-west-token
`

// nodefaultFixture mirrors F-NODEFAULT: root credentials plus east and west,
// no explicit profiles.default.
const nodefaultFixture = `endpoint: https://root.invalid/api/v2
token: dummy-root-token
profiles:
  east:
    endpoint: https://east.invalid/api/v2
    token: dummy-east-token
  west:
    endpoint: https://west.invalid/api/v2
    token: dummy-west-token
`

// rootOnlyFixture mirrors F-ROOT-ONLY: root credentials, no profiles block.
const rootOnlyFixture = `endpoint: https://root.invalid/api/v2
token: dummy-root-token
`

// alphaFixture is a second config with a different profile population, so the
// selected-config rule (candidates come from the config in effect) is
// observable: `--config` must switch the candidate set away from the env one.
const alphaFixture = `endpoint: https://root.invalid/api/v2
profiles:
  alpha:
    endpoint: https://alpha.invalid/api/v2
    token: dummy-alpha-token
`

// writeFixture writes raw YAML to a fresh file and returns its path.
func writeFixture(t *testing.T, raw string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "drconfig.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	return path
}

// runComplete executes `dr __complete <args...>` against a fresh isolated
// root tree (no-op config initializer, TLS, telemetry, animation, and plugin
// discovery) and returns stdout and stderr. Candidates come from the
// completion functions themselves, which resolve the selected config
// independently of the pre-run.
func runComplete(t *testing.T, args ...string) (string, string) {
	t.Helper()

	root := cmd.NewIsolatedRootFactory().Build()

	var out, errOut bytes.Buffer

	root.SetOut(&out)
	root.SetErr(&errOut)

	root.SetArgs(append([]string{"__complete"}, args...))
	require.NoError(t, root.Execute(), "__complete must always succeed, even with zero candidates")

	return out.String(), errOut.String()
}

func TestComplete_ProfileFlag(t *testing.T) {
	t.Run("offers explicit names only", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, nodefaultFixture))

		out, _ := runComplete(t, "--profile", "")

		assert.Equal(t, "east\nwest\n:4\n", out,
			"the flag candidates must be exactly the defined names, no default, no file completion")
	})

	t.Run("includes default only when explicitly defined", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "--profile", "")

		assert.Contains(t, out, "default\n")
		assert.Contains(t, out, "east\n")
		assert.True(t, strings.HasSuffix(out, ":4\n"), "directive must be no-file-completion")
	})

	t.Run("prefix filters", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, nodefaultFixture))

		out, _ := runComplete(t, "auth", "check", "--profile", "w")

		assert.Equal(t, "west\n:4\n", out)
	})

	t.Run("root-only config yields no candidates", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, rootOnlyFixture))

		out, _ := runComplete(t, "--profile", "")

		assert.Equal(t, ":4\n", out)
	})

	t.Run("candidates follow the selected config", func(t *testing.T) {
		// The env selects the alpha config, but an explicit --config inside
		// the completed command line must win, mirroring the pre-run's
		// resolution order.
		envConfig := writeFixture(t, nodefaultFixture)
		flagConfig := writeFixture(t, alphaFixture)
		t.Setenv("DATAROBOT_CLI_CONFIG", envConfig)

		envSelected, _ := runComplete(t, "--profile", "")
		assert.Contains(t, envSelected, "east\n", "the env-selected config's names must be offered")

		flagSelected, _ := runComplete(t, "--config", flagConfig, "--profile", "")
		assert.Equal(t, "alpha\n:4\n", flagSelected,
			"the --config selected config's names must be offered when given")
	})
}

func TestComplete_ShowPositional(t *testing.T) {
	t.Run("offers defined names", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, nodefaultFixture))

		out, _ := runComplete(t, "auth", "profile", "show", "")

		assert.Equal(t, "east\nwest\n:4\n", out)
	})

	t.Run("includes explicit default when defined", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "show", "")

		assert.Contains(t, out, "default\n")
	})

	t.Run("prefix filters", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, nodefaultFixture))

		out, _ := runComplete(t, "auth", "profile", "show", "e")

		assert.Equal(t, "east\n:4\n", out)
	})

	t.Run("no candidates after one name", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, nodefaultFixture))

		out, _ := runComplete(t, "auth", "profile", "show", "east", "")

		assert.Equal(t, ":4\n", out, "show accepts one name, so nothing is left to suggest")
	})

	t.Run("root-only config yields no candidates", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, rootOnlyFixture))

		out, _ := runComplete(t, "auth", "profile", "show", "")

		assert.Equal(t, ":4\n", out)
	})
}

func TestComplete_DeletePositional(t *testing.T) {
	t.Run("offers deletable names without reserved default", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "delete", "")

		assert.Equal(t, "east\nendpoint-only\nwest\n:4\n", out,
			"delete suggestions must never imply the reserved default is deletable")
	})

	t.Run("rm yields the same candidates as delete", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "rm", "")

		assert.Equal(t, "east\nendpoint-only\nwest\n:4\n", out)
	})

	t.Run("prefix filters", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "delete", "e")

		assert.Equal(t, "east\nendpoint-only\n:4\n", out)
	})

	t.Run("no candidates after one name", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "delete", "east", "")

		assert.Equal(t, ":4\n", out)
	})

	t.Run("rm has no candidates after one name", func(t *testing.T) {
		t.Setenv("DATAROBOT_CLI_CONFIG", writeFixture(t, bothFixture))

		out, _ := runComplete(t, "auth", "profile", "rm", "east", "")

		assert.Equal(t, ":4\n", out)
	})
}

// TestComplete_NoSideEffects pins the contract that completion never mutates
// the selected config: every probe in this suite runs against one fixture
// whose bytes are unchanged afterward.
func TestComplete_NoSideEffects(t *testing.T) {
	fixture := writeFixture(t, bothFixture)
	t.Setenv("DATAROBOT_CLI_CONFIG", fixture)

	before := sha256.Sum256([]byte(bothFixture))

	for _, args := range [][]string{
		{"--profile", ""},
		{"auth", "profile", "show", ""},
		{"auth", "profile", "show", "eas"},
		{"auth", "profile", "delete", ""},
		{"auth", "profile", "rm", ""},
	} {
		_, errOut := runComplete(t, args...)
		assert.NotContains(t, errOut, "Error", "completion probes must not error")
	}

	after, err := os.ReadFile(fixture)
	require.NoError(t, err)

	afterSum := sha256.Sum256(after)

	assert.Equal(t, before[:], afterSum[:], "completion must leave the config file byte-identical")
}
