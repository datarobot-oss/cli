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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	wldoctor "github.com/datarobot/cli/internal/workload/doctor"
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

// withOfferRelink overrides the interactive relink offer seam.
func withOfferRelink(t *testing.T, fn func(io.Writer, string) (string, error)) {
	t.Helper()

	orig := offerRelinkFn
	offerRelinkFn = fn

	t.Cleanup(func() { offerRelinkFn = orig })
}

// withRelinkConfirm overrides the relink confirm builder seam.
func withRelinkConfirm(t *testing.T, fn func(*cobra.Command) wldoctor.RelinkConfirmFunc) {
	t.Helper()

	orig := makeRelinkConfirmFn
	makeRelinkConfirmFn = fn

	t.Cleanup(func() { makeRelinkConfirmFn = orig })
}

// withInteractive forces the interactive path (or non-interactive) by
// overriding the isInteractiveFn seam.
func withInteractive(t *testing.T, interactive bool) {
	t.Helper()

	orig := isInteractiveFn

	if interactive {
		isInteractiveFn = func(*cobra.Command) bool { return true }
	} else {
		isInteractiveFn = func(*cobra.Command) bool { return false }
	}

	t.Cleanup(func() { isInteractiveFn = orig })
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

// runCapture executes cmd with captured stdout and stderr buffers wired via
// cmd.SetOut/SetErr so tests can inspect both streams independently.
func runCapture(t *testing.T, cmd *cobra.Command) (stdout, stderr string, err error) {
	t.Helper()

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.SetOut(&stdoutBuf)
	cmd.SetErr(&stderrBuf)

	err = cmd.Execute()

	return stdoutBuf.String(), stderrBuf.String(), err
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

	historyData, err := os.ReadFile(filepath.Join(wapi.Dir(tmp), wapi.HistoryFile))
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

	assert.False(t, wapi.Exists(tmp), "state dir must not be created when artifact is locked")
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

	// The linked artifact is healthy (exists, not locked, no catalog mismatch
	// since config has no catalogId).
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "existing-art", "DRAFT", nil), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-new-id"})

	out := captureStdout(t, func() {
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "init aborted")
	})

	assert.Contains(t, out, "Already linked to artifact art-existing-999")
	assert.Contains(t, out, "dr artifact code doctor")
	assert.NotContains(t, out, "Delete")
	assert.NotContains(t, out, "re-init")
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
	assert.True(t, wapi.Exists(tmp), "state dir must be created in non-interactive mode via env var")
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

// ---------------------------------------------------------------------------
// Already-linked branches (VAL-INIT-001 through VAL-INIT-016)
// ---------------------------------------------------------------------------

// TestRunE_AlreadyLinked_GoneArtifact_NonInteractive covers VAL-INIT-005:
// gone artifact (404) in non-interactive mode prints guidance naming
// doctor --relink, never delete advice, state byte-identical.
func TestRunE_AlreadyLinked_GoneArtifact_NonInteractive(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-001",
	}))

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-gone-001"})

	stdout, _, err := runCapture(t, cmd)

	require.Error(t, err)
	assert.Contains(t, stdout, "art-gone-001")
	assert.Contains(t, stdout, "dr artifact code doctor --relink <new-artifact-id>")
	assert.NotContains(t, stdout, "Delete")
	assert.NotContains(t, stdout, "rm -rf")
	assert.NotContains(t, stdout, "re-init")

	// State byte-identical (no relink performed).
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-gone-001", cfg.ArtifactID)
}

// TestRunE_AlreadyLinked_CatalogMismatch_NonInteractive covers VAL-INIT-008:
// catalog mismatch in non-interactive mode points to doctor --relink, no
// delete advice, state unchanged.
func TestRunE_AlreadyLinked_CatalogMismatch_NonInteractive(t *testing.T) {
	tmp := t.TempDir()

	catA := "cat-original-001"
	catB := "cat-mismatch-002"

	require.NoError(t, os.MkdirAll(wapi.Dir(tmp), 0o755))

	require.NoError(t, wapi.SaveConfig(tmp, wapi.Config{
		ArtifactID: "art-mismatch-001",
		CatalogID:  &catB, // hand-edited to a bogus value
		CreatedAt:  time.Now().UTC(),
		CLIVersion: "test",
	}))
	require.NoError(t, wapi.SaveManifest(tmp, wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{},
	}))

	// The artifact exists but its codeRef.CatalogID is catA (≠ config's catB).
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "mismatch-art", "DRAFT", &workload.DatarobotCodeRef{
			CatalogID:        catA,
			CatalogVersionID: "ver-001",
		}), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-mismatch-001"})

	stdout, _, err := runCapture(t, cmd)

	require.Error(t, err)
	assert.Contains(t, stdout, "catalog id no longer matches")
	assert.Contains(t, stdout, "dr artifact code doctor --relink <new-artifact-id>")
	assert.NotContains(t, stdout, "Delete")
	assert.NotContains(t, stdout, "re-init")

	// State unchanged.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-mismatch-001", cfg.ArtifactID)
	require.NotNil(t, cfg.CatalogID)
	assert.Equal(t, catB, *cfg.CatalogID)
}

// TestRunE_AlreadyLinked_CorruptConfig covers VAL-INIT-015:
// corrupt config (unreadable linked state) reports unreadable, remedy names
// doctor --fix, never deletion, no fetch, state byte-identical.
func TestRunE_AlreadyLinked_CorruptConfig(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(wapi.Dir(tmp), 0o755))

	// Write a corrupt config.json.
	require.NoError(t, os.WriteFile(wapi.ConfigPath(tmp), []byte(`{"artifactId":"abc`), 0o600))

	// getArtifactFn must NOT be called (config is unreadable, no fetch).
	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		t.Fatal("getArtifactFn must not be called when config is corrupt")

		return nil, nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-some-id"})

	stdout, _, err := runCapture(t, cmd)

	require.Error(t, err)
	assert.Contains(t, stdout, "unreadable")
	assert.Contains(t, stdout, "dr artifact code doctor --fix")
	assert.NotContains(t, stdout, "Delete")
	assert.NotContains(t, stdout, "rm -rf")
	assert.NotContains(t, stdout, "re-init")

	// State byte-identical (config still corrupt).
	_, statErr := os.Stat(wapi.ConfigPath(tmp))
	require.NoError(t, statErr)
}

// TestRunE_AlreadyLinked_Healthy_JSON covers VAL-INIT-002:
// healthy linked artifact in JSON mode emits the pinned abort shape on stdout
// with human text on stderr, exit 1, no delete advice.
func TestRunE_AlreadyLinked_Healthy_JSON(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-healthy-001",
	}))

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "healthy-art", "DRAFT", nil), nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-other-id"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	stdout, stderr, err := runCapture(t, cmd)

	require.Error(t, err)

	// stdout is pure JSON with the pinned shape.
	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, "error", parsed["status"])
	assert.Equal(t, "already-linked", parsed["error"])
	assert.Equal(t, "art-healthy-001", parsed["artifactId"])
	assert.Contains(t, parsed["remedy"], "dr artifact code doctor")

	// stderr has human text, no delete advice.
	assert.Contains(t, stderr, "Already linked to artifact art-healthy-001")
	assert.NotContains(t, stderr, "Delete")
	assert.NotContains(t, stderr, "re-init")
}

// TestRunE_AlreadyLinked_Gone_JSON covers VAL-INIT-006:
// gone artifact in JSON mode emits the pinned shape with remedy containing
// doctor --relink, human text to stderr, no deletion, exit non-zero, state
// unchanged.
func TestRunE_AlreadyLinked_Gone_JSON(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-002",
	}))

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-gone-002"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	stdout, stderr, err := runCapture(t, cmd)

	require.Error(t, err)

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, "error", parsed["status"])
	assert.Equal(t, "already-linked", parsed["error"])
	assert.Equal(t, "art-gone-002", parsed["artifactId"])
	assert.Contains(t, parsed["remedy"], "doctor --relink")

	assert.NotContains(t, stderr, "Delete")
	assert.NotContains(t, stderr, "re-init")

	// State unchanged.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-gone-002", cfg.ArtifactID)
}

// TestRunE_AlreadyLinked_CorruptConfig_JSON covers the corrupt-config branch
// in JSON mode: pinned shape with null artifactId, remedy doctor --fix.
func TestRunE_AlreadyLinked_CorruptConfig_JSON(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(wapi.Dir(tmp), 0o755))
	require.NoError(t, os.WriteFile(wapi.ConfigPath(tmp), []byte(`{"artifactId":"abc`), 0o600))

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		t.Fatal("getArtifactFn must not be called when config is corrupt")

		return nil, nil
	})

	cmd := newTestCmd(t, tmp, true, []string{"art-some-id"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	stdout, stderr, err := runCapture(t, cmd)

	require.Error(t, err)

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, "error", parsed["status"])
	assert.Equal(t, "already-linked", parsed["error"])
	assert.Nil(t, parsed["artifactId"])
	assert.Contains(t, parsed["remedy"], "doctor --fix")

	assert.NotContains(t, stderr, "Delete")
	assert.NotContains(t, stderr, "re-init")
}

// TestRunE_AlreadyLinked_GoneArtifact_RelinkAccept covers VAL-INIT-003:
// interactive gone-artifact offer accepted drives the full relink (config
// repointed, manifest reset, relink history entry), working tree untouched.
func TestRunE_AlreadyLinked_GoneArtifact_RelinkAccept(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-003",
	}))

	// The linked artifact is gone (404); the new artifact exists and is healthy.
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		if id == "art-gone-003" {
			return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
		}

		return fakeArtifact(id, "new-art", "DRAFT", nil), nil
	})

	// Force the interactive path.
	withInteractive(t, true)

	// Simulate the user accepting the offer and entering a new artifact ID.
	withOfferRelink(t, func(_ io.Writer, notice string) (string, error) {
		assert.Contains(t, notice, "not found")

		return "art-new-003", nil
	})

	// Simulate the user confirming the relink.
	withRelinkConfirm(t, func(_ *cobra.Command) wldoctor.RelinkConfirmFunc {
		return func(_ string) bool { return true }
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-gone-003"})

	stdout, _, err := runCapture(t, cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Relinked to artifact art-new-003")

	// Config repointed.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-new-003", cfg.ArtifactID)
	assert.Nil(t, cfg.LastSyncedVersionID)

	// Manifest reset to empty BASE.
	manifest, manifestErr := wapi.LoadManifest(tmp)
	require.NoError(t, manifestErr)
	assert.Empty(t, manifest.Files)
	assert.Nil(t, manifest.SyncedVersionID)

	// History has a relink entry.
	historyData, historyErr := os.ReadFile(filepath.Join(wapi.Dir(tmp), wapi.HistoryFile))
	require.NoError(t, historyErr)
	assert.Contains(t, string(historyData), `"op":"relink"`)
	assert.Contains(t, string(historyData), `"from":"art-gone-003"`)
	assert.Contains(t, string(historyData), `"to":"art-new-003"`)
}

// TestRunE_AlreadyLinked_GoneArtifact_RelinkDecline covers VAL-INIT-004:
// interactive gone-artifact offer declined leaves state byte-identical.
func TestRunE_AlreadyLinked_GoneArtifact_RelinkDecline(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-004",
	}))

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
	})

	withInteractive(t, true)

	// Simulate the user declining the offer.
	withOfferRelink(t, func(_ io.Writer, _ string) (string, error) {
		return "", nil // declined
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-gone-004"})

	_, _, err := runCapture(t, cmd)

	require.Error(t, err)

	// State byte-identical.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-gone-004", cfg.ArtifactID)

	// No relink history entry.
	historyData, historyErr := os.ReadFile(filepath.Join(wapi.Dir(tmp), wapi.HistoryFile))
	require.NoError(t, historyErr)
	assert.NotContains(t, string(historyData), `"op":"relink"`)
}

// TestRunE_AlreadyLinked_GoneArtifact_Relink404Target covers VAL-INIT-012:
// interactive offer accepted but the new artifact ID 404s → abort, state
// untouched.
func TestRunE_AlreadyLinked_GoneArtifact_Relink404Target(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-005",
	}))

	// Both the old and new artifact IDs 404.
	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
	})

	withInteractive(t, true)

	withOfferRelink(t, func(_ io.Writer, _ string) (string, error) {
		return "art-also-gone-001", nil
	})

	withRelinkConfirm(t, func(_ *cobra.Command) wldoctor.RelinkConfirmFunc {
		return func(_ string) bool { return true }
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-gone-005"})

	_, stderr, err := runCapture(t, cmd)

	require.Error(t, err)
	assert.Contains(t, stderr, "not found")

	// State untouched.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-gone-005", cfg.ArtifactID)

	historyData, historyErr := os.ReadFile(filepath.Join(wapi.Dir(tmp), wapi.HistoryFile))
	require.NoError(t, historyErr)
	assert.NotContains(t, string(historyData), `"op":"relink"`)
}

// TestRunE_AlreadyLinked_GoneArtifact_RelinkLockedTarget covers
// VAL-INIT-013: interactive offer accepted but the new artifact is locked →
// abort, state untouched.
func TestRunE_AlreadyLinked_GoneArtifact_RelinkLockedTarget(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-006",
	}))

	// Old artifact 404; new artifact is locked.
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		if id == "art-gone-006" {
			return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
		}

		return fakeArtifact(id, "locked-target", "LOCKED", nil), nil
	})

	withInteractive(t, true)

	withOfferRelink(t, func(_ io.Writer, _ string) (string, error) {
		return "art-locked-target-001", nil
	})

	withRelinkConfirm(t, func(_ *cobra.Command) wldoctor.RelinkConfirmFunc {
		return func(_ string) bool { return true }
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-gone-006"})

	_, stderr, err := runCapture(t, cmd)

	require.Error(t, err)
	assert.Contains(t, stderr, "locked")

	// State untouched.
	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-gone-006", cfg.ArtifactID)
}

// TestRunE_AlreadyLinked_CatalogMismatch_RelinkAccept covers VAL-INIT-007:
// catalog mismatch interactive offer accepted drives the full relink.
func TestRunE_AlreadyLinked_CatalogMismatch_RelinkAccept(t *testing.T) {
	tmp := t.TempDir()

	catA := "cat-original-003"
	catB := "cat-mismatch-003"

	require.NoError(t, os.MkdirAll(wapi.Dir(tmp), 0o755))

	require.NoError(t, wapi.SaveConfig(tmp, wapi.Config{
		ArtifactID: "art-mismatch-003",
		CatalogID:  &catB, // mismatched
		CreatedAt:  time.Now().UTC(),
		CLIVersion: "test",
	}))
	require.NoError(t, wapi.SaveManifest(tmp, wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{},
	}))

	// The artifact exists but its codeRef.CatalogID is catA (≠ config's catB).
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		if id == "art-mismatch-003" {
			return fakeArtifact(id, "mismatch-art", "DRAFT", &workload.DatarobotCodeRef{
				CatalogID:        catA,
				CatalogVersionID: "ver-001",
			}), nil
		}

		return fakeArtifact(id, "new-art", "DRAFT", nil), nil
	})

	withInteractive(t, true)

	withOfferRelink(t, func(_ io.Writer, notice string) (string, error) {
		assert.Contains(t, notice, "mismatch")

		return "art-new-mismatch-001", nil
	})

	withRelinkConfirm(t, func(_ *cobra.Command) wldoctor.RelinkConfirmFunc {
		return func(_ string) bool { return true }
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-mismatch-003"})

	stdout, _, err := runCapture(t, cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Relinked to artifact art-new-mismatch-001")

	cfg, cfgErr := wapi.LoadConfig(tmp)
	require.NoError(t, cfgErr)
	assert.Equal(t, "art-new-mismatch-001", cfg.ArtifactID)
}

// TestRunE_AlreadyLinked_RelinkJSON covers VAL-INIT-011:
// interactive relink offer in JSON mode — stdout is pure JSON describing the
// relink result, prompts/warnings to stderr, exit 0.
func TestRunE_AlreadyLinked_RelinkJSON(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{
		ArtifactID: "art-gone-007",
	}))

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		if id == "art-gone-007" {
			return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
		}

		return fakeArtifact(id, "new-art", "DRAFT", nil), nil
	})

	withInteractive(t, true)

	withOfferRelink(t, func(_ io.Writer, _ string) (string, error) {
		return "art-new-007", nil
	})

	withRelinkConfirm(t, func(_ *cobra.Command) wldoctor.RelinkConfirmFunc {
		return func(_ string) bool { return true }
	})

	cmd := newTestCmd(t, tmp, false, []string{"art-gone-007"})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	stdout, _, err := runCapture(t, cmd)

	require.NoError(t, err)

	// stdout is pure JSON.
	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	assert.Equal(t, "ok", parsed["status"])
	assert.Equal(t, "art-new-007", parsed["artifactId"])
}

// TestRunE_AlreadyLinked_NoDeleteAdvice is the grep guard (VAL-INIT-014):
// no init output path advises deleting .datarobot/workload/ or .wapi state.
func TestRunE_AlreadyLinked_NoDeleteAdvice(t *testing.T) {
	badSubstrings := []string{"Delete ", "rm -rf", "remove the state", "to re-init"}

	// Branch 1: healthy abort (text).
	t.Run("healthy_text", func(t *testing.T) {
		tmp := t.TempDir()

		require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{ArtifactID: "art-1"}))

		withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
			return fakeArtifact(id, "h", "DRAFT", nil), nil
		})

		cmd := newTestCmd(t, tmp, true, []string{"art-x"})

		stdout, stderr, _ := runCapture(t, cmd)

		for _, bad := range badSubstrings {
			assert.NotContains(t, stdout, bad, "stdout must not contain %q", bad)
			assert.NotContains(t, stderr, bad, "stderr must not contain %q", bad)
		}
	})

	// Branch 2: gone guidance (text).
	t.Run("gone_text", func(t *testing.T) {
		tmp := t.TempDir()

		require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{ArtifactID: "art-2"}))

		withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
			return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
		})

		cmd := newTestCmd(t, tmp, true, []string{"art-2"})

		stdout, stderr, _ := runCapture(t, cmd)

		for _, bad := range badSubstrings {
			assert.NotContains(t, stdout, bad, "stdout must not contain %q", bad)
			assert.NotContains(t, stderr, bad, "stderr must not contain %q", bad)
		}
	})

	// Branch 3: corrupt config (text).
	t.Run("corrupt_text", func(t *testing.T) {
		tmp := t.TempDir()

		require.NoError(t, os.MkdirAll(wapi.Dir(tmp), 0o755))
		require.NoError(t, os.WriteFile(wapi.ConfigPath(tmp), []byte(`{`), 0o600))

		withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
			return nil, nil
		})

		cmd := newTestCmd(t, tmp, true, []string{"art-3"})

		stdout, stderr, _ := runCapture(t, cmd)

		for _, bad := range badSubstrings {
			assert.NotContains(t, stdout, bad, "stdout must not contain %q", bad)
			assert.NotContains(t, stderr, bad, "stderr must not contain %q", bad)
		}
	})

	// Branch 4: healthy abort (JSON).
	t.Run("healthy_json", func(t *testing.T) {
		tmp := t.TempDir()

		require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{ArtifactID: "art-4"}))

		withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
			return fakeArtifact(id, "h", "DRAFT", nil), nil
		})

		cmd := newTestCmd(t, tmp, true, []string{"art-x"})
		require.NoError(t, cmd.Flags().Set("output-format", "json"))

		stdout, stderr, _ := runCapture(t, cmd)

		for _, bad := range badSubstrings {
			assert.NotContains(t, stdout, bad, "stdout must not contain %q", bad)
			assert.NotContains(t, stderr, bad, "stderr must not contain %q", bad)
		}
	})

	// Branch 5: gone guidance (JSON).
	t.Run("gone_json", func(t *testing.T) {
		tmp := t.TempDir()

		require.NoError(t, wapi.Initialize(tmp, wapi.InitOptions{ArtifactID: "art-5"}))

		withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
			return nil, &drapi.HTTPError{StatusCode: 404, URL: "test"}
		})

		cmd := newTestCmd(t, tmp, true, []string{"art-5"})
		require.NoError(t, cmd.Flags().Set("output-format", "json"))

		stdout, stderr, _ := runCapture(t, cmd)

		for _, bad := range badSubstrings {
			assert.NotContains(t, stdout, bad, "stdout must not contain %q", bad)
			assert.NotContains(t, stderr, bad, "stderr must not contain %q", bad)
		}
	})
}
