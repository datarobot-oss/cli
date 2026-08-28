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

package doctor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testArtifactID matches the bare-hex shape of real DataRobot artifact ids.
	testArtifactID = "6a90da2ddeadbeefcafe1234"

	// testHash is a syntactically valid SHA-256 hex digest for manifest fixtures.
	testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// jsonSummary mirrors the pinned summary block of the doctor's JSON report.
type jsonSummary struct {
	OK   int `json:"ok"`
	WARN int `json:"warn"`
	FAIL int `json:"fail"`
	SKIP int `json:"skip"`
}

// jsonCheck mirrors one element of the pinned checks array.
type jsonCheck struct {
	ID      string            `json:"id"`
	Status  string            `json:"status"`
	Summary string            `json:"summary"`
	Remedy  string            `json:"remedy"`
	Details map[string]string `json:"details"`
	Fixable bool              `json:"fixable"`
}

// jsonReport mirrors the pinned top-level doctor JSON schema.
type jsonReport struct {
	ProjectDir string      `json:"projectDir"`
	ArtifactID *string     `json:"artifactId"`
	Status     string      `json:"status"`
	Checks     []jsonCheck `json:"checks"`
	Summary    jsonSummary `json:"summary"`
}

// pinnedCheckOrder is the fixed check order the command must preserve:
// six local checks then the four remote checks (ten total).
var pinnedCheckOrder = []string{
	"wapi.presence",
	"wapi.config",
	"wapi.manifest",
	"wapi.config-manifest-divergence",
	"wapi.rollback",
	"wapi.lock",
	"remote.artifact-exists",
	"remote.artifact-locked",
	"remote.catalog-mismatch",
	"remote.drift",
}

// withFakeArtifact swaps the command's remote artifact seam for fn, restoring
// the original when the test ends.
func withFakeArtifact(t *testing.T, fn func(string) (*workload.Artifact, error)) {
	t.Helper()

	orig := getArtifactFn

	getArtifactFn = fn

	t.Cleanup(func() { getArtifactFn = orig })
}

// fakeArtifact builds an artifact fixture with an optional codeRef planted on
// the primary container (mirrors the init command's test helper).
func fakeArtifact(id, name, status string, codeRef *workload.DatarobotCodeRef) *workload.Artifact {
	art := &workload.Artifact{
		ID:     id,
		Name:   name,
		Status: status,
	}

	if codeRef == nil {
		return art
	}

	primary := true

	art.Spec.ContainerGroups = []workload.ContainerGroup{
		{
			Containers: []workload.Container{
				{
					Primary: &primary,

					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{Datarobot: codeRef},
					},
				},
			},
		},
	}

	return art
}

// newTestCmd builds the doctor command with buffered stdout/stderr and no
// ambient arguments.
func newTestCmd(t *testing.T, args ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	c := Cmd()

	c.SetArgs(args)

	out := &bytes.Buffer{}

	errOut := &bytes.Buffer{}

	c.SetOut(out)
	c.SetErr(errOut)

	return c, out, errOut
}

// writeStateFile writes raw contents to a file inside the project's state
// directory (creating any missing parent directories first). wapi.Dir
// resolves the legacy location when only a legacy directory exists.
func writeStateFile(t *testing.T, projectDir, name, contents string) {
	t.Helper()

	target := filepath.Join(wapi.Dir(projectDir), name)

	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))

	require.NoError(t, os.WriteFile(target, []byte(contents), 0o600))
}

// linkHealthyProject hand-crafts a fully healthy never-synced state: valid
// config (no catalog pointers) plus a valid manifest (empty BASE).
func linkHealthyProject(t *testing.T, projectDir string) {
	t.Helper()

	writeStateFile(t, projectDir, "config.json",
		`{"artifactId":"`+testArtifactID+`","createdAt":"2026-01-01T00:00:00Z","cliVersion":"test-version"}`)

	writeStateFile(t, projectDir, "manifest.json",
		`{"version":1,"syncedAt":null,"syncedVersionId":null,"files":{"app/main.go":{"hash":"`+testHash+`","size":3}}}`)
}

// stateFileHashes maps every file under projectDir to its SHA-256 hex digest,
// so tests can prove a read-only run wrote nothing and created no files.
// Reads go through an os.Root scoped to projectDir so the walk cannot be
// raced into following a symlink outside the project.
func stateFileHashes(t *testing.T, projectDir string) map[string]string {
	t.Helper()

	root, err := os.OpenRoot(projectDir)

	require.NoError(t, err)

	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Logf("close project root: %v", closeErr)
		}
	}()

	hashes := map[string]string{}

	err = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(projectDir, path)
		if err != nil {
			return err
		}

		f, err := root.Open(rel)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(f)

		if closeErr := f.Close(); closeErr != nil {
			return closeErr
		}

		if err != nil {
			return err
		}

		sum := sha256.Sum256(data)

		hashes[path] = hex.EncodeToString(sum[:])

		return nil
	})

	require.NoError(t, err)

	return hashes
}

// mustRun executes the command and returns its rendered stdout.
func mustRun(t *testing.T, c *cobra.Command, out *bytes.Buffer) string {
	t.Helper()

	require.NoError(t, c.Execute())

	return out.String()
}

func TestRunE_HealthyProject_TextReport_ExitZero(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	// A never-synced draft (no codeRef) matches the never-synced state files:
	// every check, local and remote, must be OK.
	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, errOut := newTestCmd(t, "--dir", tmp)

	outStr := mustRun(t, c, out)

	assert.Empty(t, errOut.String())
	assert.Contains(t, outStr, tmp, "header names the absolute project dir")
	assert.Contains(t, outStr, testArtifactID, "header names the linked artifact")
	assert.Contains(t, outStr, "CHECK")
	assert.Contains(t, outStr, "STATUS")
	assert.Contains(t, outStr, "DETAIL")

	for _, id := range pinnedCheckOrder {
		assert.Contains(t, outStr, id, "renders check row %s", id)
	}

	assert.Contains(t, outStr, "Summary: 10 ok, 0 warn, 0 fail, 0 skip — verdict: ok")
}

func TestRunE_UnlinkedProject_RendersReport_ExitsOneSilently(t *testing.T) {
	tmp := t.TempDir()

	c, out, errOut := newTestCmd(t, "--dir", tmp)

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "any FAIL exits 1 via the silent sentinel")

	assert.Contains(t, out.String(), "wapi.presence")
	assert.Contains(t, out.String(), "FAIL")
	assert.Contains(t, out.String(), "dr artifact code init <artifact-id>")
	assert.NotContains(t, errOut.String(), "Error:", "no cobra error echo after the rendered report")
}

func TestRunE_HealthyProject_JSONReport(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--output-format", "json")

	outStr := mustRun(t, c, out)

	var report jsonReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report), "stdout is a single pure-JSON object")

	assert.Equal(t, tmp, report.ProjectDir)
	require.NotNil(t, report.ArtifactID)
	assert.Equal(t, testArtifactID, *report.ArtifactID)
	assert.Equal(t, "ok", report.Status)
	require.Len(t, report.Checks, len(pinnedCheckOrder))

	gotOrder := make([]string, 0, len(report.Checks))

	for _, check := range report.Checks {
		gotOrder = append(gotOrder, check.ID)
		assert.Equal(t, "OK", check.Status, "check %s", check.ID)
	}

	assert.Equal(t, pinnedCheckOrder, gotOrder)
	assert.Equal(t, jsonSummary{OK: 10}, report.Summary)
}

func TestRunE_JSONOutput_CorruptConfig_FailWithPath(t *testing.T) {
	tmp := t.TempDir()

	writeStateFile(t, tmp, "config.json", `{"artifactId":"abc`)

	c, out, _ := newTestCmd(t, "--dir", tmp, "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent)

	var report jsonReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON on failure")

	assert.Equal(t, "fail", report.Status)

	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	cfg := byID["wapi.config"]

	assert.Equal(t, "FAIL", cfg.Status)

	wantPath, err := filepath.Abs(filepath.Join(wapi.Dir(tmp), "config.json"))

	require.NoError(t, err)

	assert.Equal(t, wantPath, cfg.Details["path"])
	assert.Nil(t, report.ArtifactID, "unreadable config reports artifactId null")
}

func TestRunE_DirDefaultsToCwdWithoutPrompting(t *testing.T) {
	tmp := t.TempDir()

	t.Chdir(tmp)

	linkHealthyProject(t, tmp)

	// No --dir, no --yes: the doctor must diagnose cwd without prompting.
	c, out, errOut := newTestCmd(t, "--output-format", "json")

	c.SetIn(strings.NewReader(""))

	outStr := mustRun(t, c, out)

	assert.NotContains(t, outStr+errOut.String(), "Project directory", "never reuses the dirprompt")

	var report jsonReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report))

	want, err := filepath.EvalSymlinks(tmp)

	require.NoError(t, err)

	got, err := filepath.EvalSymlinks(report.ProjectDir)

	require.NoError(t, err)

	assert.Equal(t, want, got, "projectDir is the absolute cwd")
}

func TestRunE_RelativeDirResolvedAbsolute(t *testing.T) {
	tmp := t.TempDir()

	t.Chdir(tmp)

	sub := filepath.Join(tmp, "sub")

	linkHealthyProject(t, sub)

	c, out, _ := newTestCmd(t, "--dir", "sub", "--output-format", "json")

	outStr := mustRun(t, c, out)

	var report jsonReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report))

	assert.True(t, filepath.IsAbs(report.ProjectDir), "projectDir must be absolute, got %q", report.ProjectDir)
	assert.True(t, strings.HasSuffix(report.ProjectDir, "sub"))
	assert.Equal(t, "ok", report.Status)
}

func TestRunE_SymlinkedDirResolvesToTarget(t *testing.T) {
	tmp := t.TempDir()

	target := filepath.Join(tmp, "real")

	link := filepath.Join(tmp, "link")

	linkHealthyProject(t, target)

	require.NoError(t, os.Symlink(target, link))

	c, out, _ := newTestCmd(t, "--dir", link, "--output-format", "json")

	outStr := mustRun(t, c, out)

	var report jsonReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report))

	want, err := filepath.EvalSymlinks(target)

	require.NoError(t, err)

	assert.Equal(t, want, report.ProjectDir, "a symlinked --dir reports its target")
}

// TestRunE_SymlinkedDirSameBasenameResolvesToTarget pins the fix for the
// basename-only heuristic: a symlink whose target directory shares the link's
// own basename must still be detected and resolved. The old
// filepath.Base(resolved) == filepath.Base(abs) check would treat this as a
// non-symlink because both basename components are "project".
func TestRunE_SymlinkedDirSameBasenameResolvesToTarget(t *testing.T) {
	tmp := t.TempDir()

	// Target directory shares the basename "project" with the link itself.
	target := filepath.Join(tmp, "data", "project")

	link := filepath.Join(tmp, "project")

	// linkHealthyProject creates the target dir (via MkdirAll in
	// writeStateFile) and writes valid state files into it.
	linkHealthyProject(t, target)

	require.NoError(t, os.Symlink(target, link))

	c, out, _ := newTestCmd(t, "--dir", link, "--output-format", "json")

	outStr := mustRun(t, c, out)

	var report jsonReport

	require.NoError(t, json.Unmarshal([]byte(outStr), &report))

	want, err := filepath.EvalSymlinks(target)

	require.NoError(t, err)

	assert.Equal(t, want, report.ProjectDir,
		"a symlink whose target shares its basename still resolves to the target")
}

func TestRunE_ReadOnlyRun_WritesNothing(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	// An interrupted-rollback FAIL plus the never-created sync.lock both prove
	// the read-only guarantee at the command layer.
	writeStateFile(t, tmp, ".rollback/stash/app.txt", "backed up")

	before := stateFileHashes(t, tmp)

	c, _, _ := newTestCmd(t, "--dir", tmp)

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "rollback FAIL drives exit 1")

	assert.Equal(t, before, stateFileHashes(t, tmp), "read-only run changed no file")

	_, statErr := os.Stat(filepath.Join(wapi.Dir(tmp), "sync.lock"))

	assert.True(t, os.IsNotExist(statErr), "sync.lock must not be created")
}

func TestCmd_FlagShape(t *testing.T) {
	c := Cmd()

	dirFlag := c.Flags().Lookup("dir")

	require.NotNil(t, dirFlag)
	assert.Equal(t, ".", dirFlag.DefValue, "--dir defaults to the current directory")

	yesFlag := c.Flags().Lookup(cli.YesFlagName)

	require.NotNil(t, yesFlag)
	assert.Equal(t, "y", yesFlag.Shorthand)

	assert.NotNil(t, c.Flags().Lookup("output-format"))

	assert.Contains(t, c.Annotations, "telemetry", "tracked via telemetry.TrackWith like siblings")
}

func TestCmd_RejectsPositionalArgs(t *testing.T) {
	c := Cmd()

	c.SetArgs([]string{"unexpected"})

	assert.Error(t, c.Execute())
}

func TestRunE_Remote404_DeletedArtifact_FailWithRelinkRemedy(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: 404, URL: "https://test/artifacts/x/"}
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--output-format", "json")

	err := c.Execute()

	require.ErrorIs(t, err, cli.ErrSilent, "a remote FAIL drives exit 1")

	var report jsonReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON on failure")

	require.Len(t, report.Checks, len(pinnedCheckOrder))

	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	exists := byID["remote.artifact-exists"]

	assert.Equal(t, "FAIL", exists.Status)
	assert.Contains(t, exists.Summary, "deleted")
	assert.Contains(t, exists.Remedy, "--relink")

	// The dependent remote checks SKIP rather than pile on their own FAILs.
	for _, id := range []string{"remote.artifact-locked", "remote.catalog-mismatch", "remote.drift"} {
		assert.Equal(t, "SKIP", byID[id].Status, "check %s", id)
	}

	assert.Equal(t, "fail", report.Status)
}

func TestRunE_RemoteNon404_AllSkipWithConnectivityRemedy_ExitZero(t *testing.T) {
	for name, remoteErr := range map[string]error{
		"500":             &drapi.HTTPError{StatusCode: 500, URL: "https://test/"},
		"unauthorized":    &drapi.HTTPError{StatusCode: 401, URL: "https://test/"},
		"conn-refused":    errors.New("dial tcp 127.0.0.1:443: connect: connection refused"),
		"wrapped-non-404": fmt.Errorf("fetch: %w", &drapi.HTTPError{StatusCode: 503, URL: "https://test/"}),
	} {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()

			linkHealthyProject(t, tmp)

			withFakeArtifact(t, func(_ string) (*workload.Artifact, error) {
				return nil, remoteErr
			})

			c, out, errOut := newTestCmd(t, "--dir", tmp, "--output-format", "json")

			require.NoError(t, c.Execute(), "non-404 remote errors never FAIL the run")

			var report jsonReport

			require.NoError(t, json.Unmarshal(out.Bytes(), &report), "stdout stays pure JSON")

			byID := make(map[string]jsonCheck, len(report.Checks))

			for _, check := range report.Checks {
				byID[check.ID] = check

				if strings.HasPrefix(check.ID, "remote.") {
					assert.Equal(t, "SKIP", check.Status, "check %s: summary %q", check.ID, check.Summary)

					assert.NotContains(t, check.Summary, "deleted", "non-404 is never reported as deleted")

					assert.Contains(t, check.Remedy, "auth login")

					assert.NotContains(t, check.Remedy, "--relink")
				} else {
					assert.Equal(t, "OK", check.Status, "local checks unaffected by remote failure: %s", check.ID)
				}
			}

			assert.Equal(t, jsonSummary{OK: 6, SKIP: 4}, report.Summary)

			assert.Equal(t, "ok", report.Status, "SKIP-only remote outcome keeps verdict ok")

			assert.Empty(t, errOut.String())
		})
	}
}

func TestRunE_RemoteLockedArtifact_WarnNeverFail_ExitZero(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		return fakeArtifact(id, "doctor-fixture", "locked", nil), nil
	})

	c, out, _ := newTestCmd(t, "--dir", tmp, "--output-format", "json")

	require.NoError(t, c.Execute(), "a WARN alone must not fail the run")

	var report jsonReport

	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	byID := make(map[string]jsonCheck, len(report.Checks))

	for _, check := range report.Checks {
		byID[check.ID] = check
	}

	locked := byID["remote.artifact-locked"]

	assert.Equal(t, "WARN", locked.Status, "locked must WARN, never FAIL")
	assert.False(t, locked.Fixable)
	assert.Contains(t, locked.Summary, "preview")
	assert.Contains(t, locked.Summary, "execute")
	assert.Equal(t, "warn", report.Status)
}

func TestRunE_RemoteChecks_SingleArtifactFetchPerRun(t *testing.T) {
	tmp := t.TempDir()

	linkHealthyProject(t, tmp)

	calls := 0

	withFakeArtifact(t, func(id string) (*workload.Artifact, error) {
		calls++

		return fakeArtifact(id, "doctor-fixture", "DRAFT", nil), nil
	})

	c, _, _ := newTestCmd(t, "--dir", tmp)

	require.NoError(t, c.Execute())

	assert.Equal(t, 1, calls, "all four remote checks share one artifact fetch per run")
}

func TestSoftAuthProbe(t *testing.T) {
	// Neutralize any inherited environment so each case starts from a known
	// state; t.Setenv restores them after the test.
	t.Setenv("DATAROBOT_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_ENDPOINT", "")
	t.Setenv("DATAROBOT_API_TOKEN", "")

	t.Run("complete env pair wins", func(t *testing.T) {
		t.Setenv("DATAROBOT_ENDPOINT", "https://env.example.com/api/v2")
		t.Setenv("DATAROBOT_API_TOKEN", "env-token")

		creds, ok := softAuthProbe()

		assert.True(t, ok)
		assert.Equal(t, "https://env.example.com/api/v2", creds.Endpoint)
		assert.Equal(t, "env-token", creds.Token)
	})

	t.Run("partial env pair is ignored", func(t *testing.T) {
		t.Setenv("DATAROBOT_API_TOKEN", "env-token")

		_, ok := softAuthProbe()

		assert.False(t, ok, "a lone token must not count as remote access")
	})

	t.Run("stored config used when env is silent", func(t *testing.T) {
		viperx.Set(config.DataRobotURL, "https://stored.example.com/api/v2")
		viperx.Set(config.DataRobotAPIKey, "stored-token")

		t.Cleanup(func() {
			viperx.Set(config.DataRobotURL, "")
			viperx.Set(config.DataRobotAPIKey, "")
		})

		creds, ok := softAuthProbe()

		assert.True(t, ok)
		assert.Equal(t, "https://stored.example.com", creds.Endpoint)
		assert.Equal(t, "stored-token", creds.Token)
	})

	t.Run("nothing available", func(t *testing.T) {
		_, ok := softAuthProbe()

		assert.False(t, ok)
	})
}
