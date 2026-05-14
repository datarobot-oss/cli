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

package initcmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withFakeArtifact(t *testing.T, fn func(string) (*workload.Artifact, error)) {
	t.Helper()

	orig := getArtifactFn
	getArtifactFn = fn

	t.Cleanup(func() { getArtifactFn = orig })
}

// PreRunE is removed because unit tests don't go through auth.
func newTestCmd(t *testing.T, dir string, yes bool, args []string) *cobra.Command {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	cmd.SetArgs(args)
	require.NoError(t, cmd.Flags().Set("dir", dir))

	if yes {
		require.NoError(t, cmd.Flags().Set("yes", "true"))
	}

	return cmd
}

func fakeArtifact(id, name, status string, codeRef *workload.DatarobotCodeRef) *workload.Artifact {
	art := &workload.Artifact{ID: id, Name: name, Status: status}

	if codeRef != nil {
		art.Spec = workload.Spec{
			ContainerGroups: []workload.ContainerGroup{
				{Containers: []workload.Container{
					{ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{Datarobot: codeRef},
					}},
				}},
			},
		}
	}

	return art
}

func TestCmd_TooManyArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"a", "b"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestRunE_ExistingCodeArtifact_EndToEnd(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "my-agent", "DRAFT", &workload.DatarobotCodeRef{
			CatalogID:        "cat-xyz-789",
			CatalogVersionID: "fedcba0987654321",
		}), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-abc-123"})

	out := captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	assert.Contains(t, out, "Linked to my-agent (art-abc-123) at version fedcba09.")

	cfg, err := wapi.LoadConfig(tmp)
	require.NoError(t, err)
	assert.Equal(t, "art-abc-123", cfg.ArtifactID)
	require.NotNil(t, cfg.CatalogID)
	assert.Equal(t, "cat-xyz-789", *cfg.CatalogID)
	assert.Nil(t, cfg.LastSyncedVersionID)

	manifest, err := wapi.LoadManifest(tmp)
	require.NoError(t, err)
	assert.Equal(t, wapi.ManifestVersion, manifest.Version)
	assert.Empty(t, manifest.Files)

	historyData, err := os.ReadFile(filepath.Join(tmp, wapi.DirName, wapi.HistoryFile))
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(historyData)), "\n")
	require.Len(t, lines, 1)

	var entry map[string]any

	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
	assert.Equal(t, "init", entry["op"])
	assert.Equal(t, "art-abc-123", entry["artifact"])
	assert.Equal(t, "cat-xyz-789", entry["catalog"])
}

func TestRunE_EmptyArtifact_EndToEnd(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "blank-artifact", "DRAFT", nil), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-empty-001"})

	out := captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	assert.Contains(t, out, "Linked to empty artifact blank-artifact (art-empty-001).")

	cfg, err := wapi.LoadConfig(tmp)
	require.NoError(t, err)
	assert.Equal(t, "art-empty-001", cfg.ArtifactID)
	assert.Nil(t, cfg.CatalogID)
	assert.Nil(t, cfg.LastSyncedVersionID)
}

func TestRunE_LockedArtifact(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "registered", "LOCKED", nil), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-locked-001"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifact is locked")

	assert.False(t, wapi.Exists(tmp), ".wapi/ must not be created when artifact is locked")
}

func TestRunE_NotFound(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-missing-001"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifact art-missing-001 not found")

	assert.False(t, wapi.Exists(tmp))
}

func TestRunE_AlreadyLinked(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-existing-999",
	}))

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		t.Fatal("getArtifactFn must not be called when project is already linked")

		return nil, nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-new-id"})

	out := captureStdout(t, func() {
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "init aborted")
	})

	assert.Contains(t, out, "Already linked to artifact art-existing-999")
}

func TestRunE_YesWithoutID(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		t.Fatal("getArtifactFn must not be called when --yes lacks an ID")

		return nil, nil
	})

	cmd := newTestCmd(t, tmp, true, nil)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "artifact ID is required when using --yes")
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	tmp := t.TempDir()
	cmd := newTestCmd(t, tmp, true, []string{"art-abc-123", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func TestRunE_ExistingCodeArtifact_JSONOutput(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "my-agent", "DRAFT", &workload.DatarobotCodeRef{
			CatalogID:        "cat-xyz-789",
			CatalogVersionID: "fedcba0987654321",
		}), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-abc-123"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	out := captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	var result map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "art-abc-123", result["artifactId"])
	assert.Equal(t, "my-agent", result["name"])
	assert.Equal(t, "DRAFT", result["status"])
	assert.Equal(t, "cat-xyz-789", result["catalogId"])
	assert.Equal(t, "fedcba0987654321", result["catalogVersionId"])
	assert.Equal(t, tmp, result["dir"])
}

func TestRunE_EmptyArtifact_JSONOutput(t *testing.T) {
	tmp := t.TempDir()

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "blank-artifact", "DRAFT", nil), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-empty-001"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	out := captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	var result map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, "art-empty-001", result["artifactId"])
	assert.Nil(t, result["catalogId"])
	assert.Nil(t, result["catalogVersionId"])
}

func TestRunE_DrapiNon404Passes(t *testing.T) {
	tmp := t.TempDir()

	upstream := errors.New("connection refused")

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, upstream
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-abc-123"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream)
}

func TestRunE_NonInteractiveEnvVar(t *testing.T) {
	tmp := t.TempDir()

	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "true")

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "my-agent", "DRAFT", nil), nil
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-empty-001"})

	require.NoError(t, cmd.Execute())
	assert.True(t, wapi.Exists(tmp), ".wapi/ must be created in non-interactive mode via env var")
}

// TestCmd_DoesNotClobberGlobalYesViper guards against re-introducing the
// global viper.BindPFlag("yes", ...) that would shadow cmd/dotenv's
// identically-keyed binding registered in package init().
func TestCmd_DoesNotClobberGlobalYesViper(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	cmd := Cmd()
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	assert.False(t, viperx.GetBool("yes"),
		"init's --yes must not be bound to global viper key 'yes' (would clobber dotenv)")
}
