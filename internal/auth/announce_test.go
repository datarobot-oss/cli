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

package auth

import (
	"bytes"
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
)

// seedAnnounceConfig writes a drconfig.yaml that contains a token, points
// viper at it, and returns its path. The token lets tests assert that the
// save notice never echoes credential material.
func seedAnnounceConfig(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	testutil.SetTestHomeDir(t, tempDir)

	viperx.Reset()
	t.Cleanup(viperx.Reset)

	configDir := filepath.Join(tempDir, ".config", "datarobot")
	require.NoError(t, os.MkdirAll(configDir, 0o755))

	configFile := filepath.Join(configDir, "drconfig.yaml")

	raw := "endpoint: https://app.datarobot.com/api/v2\ntoken: secret-token-value\n"
	require.NoError(t, os.WriteFile(configFile, []byte(raw), 0o600))

	viperx.SetConfigFile(configFile)
	require.NoError(t, viperx.ReadInConfig())

	return configFile
}

// TestAnnounceCredentialsSaved_WritesSingleLine pins the notice shape: one
// line naming the saved target and the config file that holds it.
func TestAnnounceCredentialsSaved_WritesSingleLine(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "drconfig.yaml")

	var out bytes.Buffer

	AnnounceCredentialsSaved(&out, "work", configPath)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")

	require.Len(t, lines, 1, "the save notice must be exactly one line")
	assert.True(t, strings.HasPrefix(lines[0], "Saved "), "the notice must announce the save: %q", lines[0])
	assert.Contains(t, lines[0], `profile "work"`, "the notice must name the saved profile")
	assert.Contains(t, lines[0], configPath, "the notice must name the config file")
}

// TestAnnounceCredentialsSavedForCommand_LabelsSelection covers the identity
// rule behind the notice: an omitted selection is announced as the
// root-level credentials (never as a profile named "default"), while an
// explicit selection -- including explicit profiles.default -- is announced
// as its normalized profile name, alongside the actual config file path.
func TestAnnounceCredentialsSavedForCommand_LabelsSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection string
		want      string
	}{
		{name: "omitted selection is the root-level credentials", selection: "", want: "the root-level credentials"},
		{name: "named profile", selection: "work", want: `profile "work"`},
		{name: "explicit default", selection: "default", want: `profile "default"`},
		{name: "normalizes the selection", selection: " WORK ", want: `profile "work"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configFile := seedAnnounceConfig(t)

			if tc.selection != "" {
				viperx.Set(config.ProfileKey, tc.selection)
			}

			var out bytes.Buffer

			cmd := &cobra.Command{Use: "test"}
			cmd.SetErr(&out)

			AnnounceCredentialsSavedForCommand(cmd)

			notice := out.String()

			assert.Contains(t, notice, tc.want, "the notice must label the saved credentials correctly")
			assert.Contains(t, notice, configFile, "the notice must name the actual config file")
			assert.NotContains(t, notice, "secret-token-value", "no token value may ever be printed")

			// Count notice lines, not the substring "Saved": the scratch
			// config path embeds this test's own name, which contains it.
			noticeLines := strings.Split(strings.TrimRight(notice, "\n"), "\n")

			require.Len(t, noticeLines, 1, "the save notice must appear exactly once, as a single line")
			assert.True(t, strings.HasPrefix(noticeLines[0], "Saved "), "the notice must announce the save: %q", noticeLines[0])
		})
	}
}

// TestAnnounceCredentialsSavedForCommand_NilErrWriter falls back to the
// process stderr when the command has no dedicated error writer, matching
// how commands capture output in production.
func TestAnnounceCredentialsSavedForCommand_NilErrWriter(t *testing.T) {
	configFile := seedAnnounceConfig(t)

	oldStderr := os.Stderr

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w

	AnnounceCredentialsSavedForCommand(&cobra.Command{Use: "test"})

	require.NoError(t, w.Close())

	os.Stderr = oldStderr

	var out bytes.Buffer

	_, err = out.ReadFrom(r)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "the root-level credentials")
	assert.Contains(t, out.String(), configFile)
}
