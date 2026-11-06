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

package seturl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// seedSetURLConfig writes a fixture drconfig.yaml with root credentials and a
// profiles.work section, selects the given profile, and returns the file path.
// --skip-auth is enabled so the tests stay offline: EnsureAuthenticatedE
// short-circuits instead of probing the endpoint.
func seedSetURLConfig(t *testing.T, profile string) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := `endpoint: https://app.datarobot.com/api/v2
token: root-token

profiles:
  work:
    endpoint: https://old-work.alife399.invalid/api/v2
`
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))

	if profile != "" {
		viperx.Set(config.ProfileKey, profile)
	}

	require.NoError(t, config.ReadConfigFile(""))

	viperx.Set(config.SkipAuthKey, true)

	return configFile
}

// runSetURL runs the set-url RunE with the given arguments, capturing the
// command's stdout and stderr writers separately.
func runSetURL(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var out, errOut bytes.Buffer

	cmd := Cmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := RunE(cmd, args)

	return out.String(), errOut.String(), err
}

// countSaveNotices counts the save-notice lines in captured output. The
// scratch config path embeds this test's own name (which contains "Saved"),
// so the count is line-based rather than substring-based.
func countSaveNotices(output string) int {
	count := 0

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Saved ") {
			count++
		}
	}

	return count
}

func readRawSetURLConfig(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var m map[string]any

	require.NoError(t, yaml.Unmarshal(data, &m))

	return m
}

// TestSetURL_ExplicitURL_AnnouncesSavedProfileOnce covers the write
// announcement: exactly one notice naming the saved profile and the actual
// config file, on the error stream so stdout stays machine-readable, after
// the endpoint landed in the profile's own section.
func TestSetURL_ExplicitURL_AnnouncesSavedProfileOnce(t *testing.T) {
	configFile := seedSetURLConfig(t, "work")

	stdout, stderr, err := runSetURL(t, "https://new-work.alife399.invalid")
	require.NoError(t, err)

	notice := stderr

	assert.Equal(t, 1, countSaveNotices(notice), "the save notice must appear exactly once")
	assert.Contains(t, notice, `profile "work"`, "the notice must name the saved profile")
	assert.Contains(t, notice, configFile, "the notice must name the actual config file")
	assert.NotContains(t, notice, "root-token", "no token value may ever be printed")
	assert.NotContains(t, stdout, "Saved", "the notice must not contaminate stdout")

	raw := readRawSetURLConfig(t, configFile)

	profiles, ok := raw["profiles"].(map[string]any)
	require.True(t, ok)

	work, ok := profiles["work"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "https://new-work.alife399.invalid/api/v2", work["endpoint"],
		"the endpoint must land in the announced profile's own section")
}

// TestSetURL_ExplicitURL_AnnouncesRootLevel covers the omitted-selection
// destination: the notice labels the write as the root-level credentials,
// never as a profile, and the endpoint lands at the top level.
func TestSetURL_ExplicitURL_AnnouncesRootLevel(t *testing.T) {
	configFile := seedSetURLConfig(t, "")

	stdout, stderr, err := runSetURL(t, "https://new-root.alife399.invalid")
	require.NoError(t, err)

	notice := stderr

	assert.Equal(t, 1, countSaveNotices(notice), "the save notice must appear exactly once")
	assert.Contains(t, notice, "the root-level credentials", "an omitted selection must be labelled as root-level")
	assert.Contains(t, notice, configFile, "the notice must name the actual config file")
	assert.NotContains(t, stdout, "Saved", "the notice must not contaminate stdout")

	raw := readRawSetURLConfig(t, configFile)

	assert.Equal(t, "https://new-root.alife399.invalid/api/v2", raw["endpoint"],
		"the endpoint must land at the top level the notice announced")
}

// TestSetURL_InvalidURL_NeverAnnouncesOrWrites pins that a rejected URL
// reports the existing error, stays silent about any save, and leaves the
// config byte-identical.
func TestSetURL_InvalidURL_NeverAnnouncesOrWrites(t *testing.T) {
	configFile := seedSetURLConfig(t, "work")

	before, err := os.ReadFile(configFile)
	require.NoError(t, err)

	stdout, stderr, runErr := runSetURL(t, "file:///invalid")

	require.ErrorIs(t, runErr, cli.ErrSilent, "a rejected URL keeps the existing silent-error contract")

	assert.NotContains(t, stderr, "Saved", "a rejected URL must not announce a save")
	assert.NotContains(t, stdout, "Saved", "a rejected URL must not announce a save")

	after, err := os.ReadFile(configFile)
	require.NoError(t, err)

	assert.Equal(t, string(before), string(after), "a rejected URL must leave the config byte-identical")
}

// TestSetURL_WriteFailure_ReportedOnceWithoutAnnouncement covers the
// unwritable-destination case the CLI cannot produce reliably end-to-end:
// the write failure is reported exactly once and names the config path, no
// save notice appears, and the file is left byte-identical.
func TestSetURL_WriteFailure_ReportedOnceWithoutAnnouncement(t *testing.T) {
	configFile := seedSetURLConfig(t, "work")

	before, err := os.ReadFile(configFile)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(configFile, 0o444))
	t.Cleanup(func() { _ = os.Chmod(configFile, 0o600) })

	// log.Error writes to the process stderr logger, not the command's error
	// writer, so both are captured for the report-once assertion.
	oldStderr := os.Stderr

	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)

	os.Stderr = w

	log.StartStderr()
	t.Cleanup(log.StopStderr)

	stdout, stderr, runErr := runSetURL(t, "https://new-work.alife399.invalid")

	require.NoError(t, w.Close())

	os.Stderr = oldStderr

	require.ErrorIs(t, runErr, cli.ErrSilent, "a failed write keeps the existing silent-error contract")

	var logged bytes.Buffer

	_, err = logged.ReadFrom(r)
	require.NoError(t, err)

	reported := stderr + logged.String()

	assert.Equal(t, 1, strings.Count(reported, "failed to write config file"),
		"the write failure must be reported exactly once")
	assert.Contains(t, reported, configFile, "the write failure must name the config path")
	assert.NotContains(t, reported, "Saved", "a failed write must not announce a save")
	assert.NotContains(t, stdout, "Saved", "a failed write must not announce a save")

	after, err := os.ReadFile(configFile)
	require.NoError(t, err)

	assert.Equal(t, string(before), string(after), "a failed write must leave the config byte-identical")
}

// TestSetURLRejectsNonHTTPSchemeArg is the pre-existing contract: an explicit
// arg with an unsupported scheme errors out instead of falling through to the
// interactive picker. (Seeded by upstream CFX-7608; the save-announcement
// tests above cover the same path's no-claim guarantee.)
func TestSetURLRejectsNonHTTPSchemeArg(t *testing.T) {
	cmd := Cmd()

	err := cmd.RunE(cmd, []string{"ftp://app.datarobot.com"})

	require.ErrorIs(t, err, cli.ErrSilent)
}
