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

package checkout

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	gosync "sync"
	"testing"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient is a minimal in-memory filesapi.Client for cmd-level tests.
// Only the three methods checkout uses are implemented; the rest panic.
// The mutex guards counters + content under any concurrent access.
type fakeClient struct {
	mu gosync.Mutex

	versions []filesapi.CatalogVersion
	content  map[string][]byte

	allFilesErr error
	downloadErr error

	allFilesCalls int
	downloadCalls int
}

func (f *fakeClient) ListVersions(_ string, _ int) ([]filesapi.CatalogVersion, error) {
	return f.versions, nil
}

func (f *fakeClient) AllFiles(_, _ string) (map[string]filesapi.FileMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.allFilesCalls++

	if f.allFilesErr != nil {
		return nil, f.allFilesErr
	}

	out := make(map[string]filesapi.FileMeta, len(f.content))

	for path, data := range f.content {
		sum := sha256.Sum256(data)
		out[path] = filesapi.FileMeta{Hash: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}

	return out, nil
}

func (f *fakeClient) DownloadFile(_, _, path string, w io.Writer) (string, int64, error) {
	f.mu.Lock()
	f.downloadCalls++
	err := f.downloadErr
	data, ok := f.content[path]
	f.mu.Unlock()

	if err != nil {
		return "", 0, err
	}

	if !ok {
		return "", 0, errors.New("not found")
	}

	n, err := w.Write(data)

	return "", int64(n), err
}

func (f *fakeClient) callCounts() (all, dl int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.allFilesCalls, f.downloadCalls
}

// Panicking stubs for the rest of filesapi.Client.
func (*fakeClient) CreateCatalog() (*filesapi.CatalogResp, error)                { panic("unused") }
func (*fakeClient) CreateStage(string) (*filesapi.StageResp, error)              { panic("unused") }
func (*fakeClient) UploadToStage(string, string, string, int64, io.Reader) error { panic("unused") }
func (*fakeClient) ApplyStage(string, string, string) (*filesapi.ApplyStageResp, error) {
	panic("unused")
}

func (*fakeClient) UploadFromZipNew(string, int64, io.Reader) (*filesapi.FromFileResp, error) {
	panic("unused")
}

func (*fakeClient) UploadFromZipExisting(string, string, string, int64, io.Reader) (*filesapi.FromFileResp, error) {
	panic("unused")
}
func (*fakeClient) PollStatus(string) (*filesapi.StatusResp, error) { panic("unused") }
func (*fakeClient) DeleteFiles(string, []string) (*filesapi.DeleteFilesResp, error) {
	panic("unused")
}

func fakeDeps(art *workload.Artifact, fc *fakeClient) Deps {
	return Deps{
		GetArtifact: func(_ string) (*workload.Artifact, error) { return art, nil },
		Files:       fc,
	}
}

func draftArtifact(id string) *workload.Artifact {
	return &workload.Artifact{ID: id, Name: "my-agent", Status: "draft"}
}

func initLinkedDir(t *testing.T, catalogID string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-abc-123"}))

	if catalogID != "" {
		cfg, err := wapi.LoadConfig(dir)
		require.NoError(t, err)

		cfg.CatalogID = &catalogID
		require.NoError(t, wapi.SaveConfig(dir, cfg))
	}

	return dir
}

func newTestCmd(t *testing.T, dir string, deps Deps, args []string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := cmdWithDeps(deps)
	cmd.PreRunE = nil

	var buf bytes.Buffer

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Flags().Set("dir", dir))

	cmd.SetArgs(args)

	return cmd, &buf
}

const (
	verA = "abcdef1234567890abcdef1234567890"
	// verAmbiguous shares verA's 8-char prefix "abcdef12", so a query for that
	// prefix matches both — resolveVersion must reject it as ambiguous.
	verAmbiguous = "abcdef12fffffffabcdef12fffffff00"
	verC         = "11112222333344445555666677778888"
)

func TestCheckout_NotLinked(t *testing.T) {
	cmd, _ := newTestCmd(t, t.TempDir(), Deps{}, []string{"abcdef12"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

func TestCheckout_NoCatalog(t *testing.T) {
	cmd, _ := newTestCmd(t, initLinkedDir(t, ""), Deps{}, []string{"abcdef12"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no code has been synced yet")
}

func TestCheckout_HappyPath_FullID(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions: []filesapi.CatalogVersion{{ID: verA}},
		content: map[string][]byte{
			"agent.py":        []byte("print('hello')\n"),
			"utils/helper.py": []byte("def helper(): return 1\n"),
		},
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	cmd, buf := newTestCmd(t, dir, deps, []string{verA})

	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "Downloading version "+verA)
	assert.Contains(t, out, "Checked out to: "+filepath.Join(".wapi", ".checkouts", verA))
	assert.Contains(t, out, "read-only snapshot")

	checkoutDir := wapi.CheckoutDir(dir, verA)
	gotAgent, err := os.ReadFile(filepath.Join(checkoutDir, "agent.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('hello')\n", string(gotAgent))

	metaData, err := os.ReadFile(filepath.Join(checkoutDir, ".checkout-meta.json"))
	require.NoError(t, err)

	var meta checkoutMeta
	require.NoError(t, json.Unmarshal(metaData, &meta))
	assert.Equal(t, verA, meta.VersionID)
	assert.Equal(t, 2, meta.FileCount)

	historyData, err := os.ReadFile(filepath.Join(dir, ".wapi", "history.log"))
	require.NoError(t, err)

	lines := bytes.Split(bytes.TrimRight(historyData, "\n"), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 2, "expected init + checkout entries")

	var entry map[string]any

	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &entry))
	assert.Equal(t, "checkout", entry["op"])
	assert.Equal(t, verA, entry["version"])
	assert.EqualValues(t, 2, entry["files"])

	// Working dir + sync state untouched.
	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)
	assert.Nil(t, cfg.LastSyncedVersionID)
}

func TestCheckout_VersionResolution(t *testing.T) {
	cases := []struct {
		name, arg, wantErr, wantID string
		versions                   []filesapi.CatalogVersion
	}{
		{"happy short prefix", "abcdef12", "", verA, []filesapi.CatalogVersion{{ID: verA}, {ID: verC}}},
		{"ambiguous", "abcdef12", "ambiguous", "", []filesapi.CatalogVersion{{ID: verA}, {ID: verAmbiguous}}},
		{"not found", "99999999", `"99999999"`, "", []filesapi.CatalogVersion{{ID: verA}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initLinkedDir(t, "cat-1")
			fc := &fakeClient{versions: tc.versions, content: map[string][]byte{"x.txt": {0}}}
			deps := fakeDeps(draftArtifact("art-abc-123"), fc)

			cmd, _ := newTestCmd(t, dir, deps, []string{tc.arg})

			err := cmd.Execute()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.DirExists(t, wapi.CheckoutDir(dir, tc.wantID))
		})
	}
}

func TestCheckout_RecheckoutRefetches(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions: []filesapi.CatalogVersion{{ID: verA}},
		content:  map[string][]byte{"a.txt": []byte("first")},
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	cmd1, _ := newTestCmd(t, dir, deps, []string{verA})
	require.NoError(t, cmd1.Execute())

	allFilesBefore, downloadBefore := fc.callCounts()

	fc.mu.Lock()
	fc.content["a.txt"] = []byte("second")
	fc.mu.Unlock()

	cmd2, buf := newTestCmd(t, dir, deps, []string{verA})
	require.NoError(t, cmd2.Execute())

	allFilesAfter, downloadAfter := fc.callCounts()

	assert.Contains(t, buf.String(), "Downloading version "+verA)
	assert.Greater(t, allFilesAfter, allFilesBefore, "AllFiles should be re-called on refetch")
	assert.Greater(t, downloadAfter, downloadBefore, "DownloadFile should be re-called on refetch")

	got, err := os.ReadFile(filepath.Join(wapi.CheckoutDir(dir, verA), "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(got))
}

func TestCheckout_JSONOutput(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions: []filesapi.CatalogVersion{{ID: verA}},
		content:  map[string][]byte{"a.txt": []byte("a"), "b.txt": []byte("bb")},
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	cmd, buf := newTestCmd(t, dir, deps, []string{verA})
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	require.NoError(t, cmd.Execute())

	var got downloadResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	assert.Equal(t, verA, got.VersionID)
	assert.Equal(t, 2, got.FileCount)
	assert.Equal(t, int64(3), got.TotalSize)
	require.Len(t, got.Files, 2)
	assert.Equal(t, "a.txt", got.Files[0].Path)
	assert.Equal(t, "b.txt", got.Files[1].Path)
}

func TestCheckout_RejectsUnsafePath(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		wantHint string
	}{
		{"parent escape", "../etc/passwd", "escapes project root"},
		{"absolute path", "/etc/passwd", "absolute path not allowed"},
		{"backslash", "evil\\path", "backslash in path"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initLinkedDir(t, "cat-1")

			fc := &fakeClient{
				versions: []filesapi.CatalogVersion{{ID: verA}},
				content:  map[string][]byte{tc.path: []byte("evil")},
			}

			deps := fakeDeps(draftArtifact("art-abc-123"), fc)

			cmd, _ := newTestCmd(t, dir, deps, []string{verA})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsafe path")
			assert.Contains(t, err.Error(), tc.wantHint)

			_, dlCalls := fc.callCounts()
			assert.Zero(t, dlCalls, "DownloadFile must not be called when validation rejects a path")

			assert.NoDirExists(t, wapi.CheckoutDir(dir, verA))
		})
	}
}

func TestCheckout_DownloadErrorRemovesPartial(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions:    []filesapi.CatalogVersion{{ID: verA}},
		content:     map[string][]byte{"a.txt": []byte("a")},
		downloadErr: errors.New("network died"),
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	cmd, _ := newTestCmd(t, dir, deps, []string{verA})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network died")

	assert.NoDirExists(t, wapi.CheckoutDir(dir, verA))
}

func TestCheckout_RecheckoutFailurePreservesOld(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions: []filesapi.CatalogVersion{{ID: verA}},
		content:  map[string][]byte{"a.txt": []byte("first")},
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	cmd1, _ := newTestCmd(t, dir, deps, []string{verA})
	require.NoError(t, cmd1.Execute())

	checkoutDir := wapi.CheckoutDir(dir, verA)
	assert.DirExists(t, checkoutDir)

	fc.mu.Lock()
	fc.downloadErr = errors.New("network died")
	fc.mu.Unlock()

	cmd2, _ := newTestCmd(t, dir, deps, []string{verA})
	require.Error(t, cmd2.Execute())

	assert.DirExists(t, checkoutDir, "old snapshot must survive a failed re-checkout")

	got, err := os.ReadFile(filepath.Join(checkoutDir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first", string(got))
}

func TestCheckout_CleanAll(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	require.NoError(t, os.MkdirAll(wapi.CheckoutDir(dir, verA), 0o755))
	require.NoError(t, os.MkdirAll(wapi.CheckoutDir(dir, verC), 0o755))

	cmd, buf := newTestCmd(t, dir, Deps{}, nil)
	require.NoError(t, cmd.Flags().Set("clean", "true"))

	require.NoError(t, cmd.Execute())

	assert.Contains(t, buf.String(), "Removed 2 checkouts")
	assert.NoDirExists(t, wapi.CheckoutsDir(dir))
}

func TestCheckout_CleanOne(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	require.NoError(t, os.MkdirAll(wapi.CheckoutDir(dir, verA), 0o755))
	require.NoError(t, os.MkdirAll(wapi.CheckoutDir(dir, verC), 0o755))

	cmd, buf := newTestCmd(t, dir, Deps{}, []string{verA})
	require.NoError(t, cmd.Flags().Set("clean", "true"))

	require.NoError(t, cmd.Execute())

	assert.Contains(t, buf.String(), "Removed checkout "+verA)
	assert.NoDirExists(t, wapi.CheckoutDir(dir, verA))
	assert.DirExists(t, wapi.CheckoutDir(dir, verC))
}

func TestCheckout_CleanAllJSONEmptyArray(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	cmd, buf := newTestCmd(t, dir, Deps{}, nil)
	require.NoError(t, cmd.Flags().Set("clean", "true"))
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	require.NoError(t, cmd.Execute())

	var got cleanResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.NotNil(t, got.Removed, "Removed must serialize as [] not null")
	assert.Empty(t, got.Removed)
}

func TestCheckout_YesWithoutVersionErrors(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	cmd, _ := newTestCmd(t, dir, Deps{}, nil)
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version argument is required")
}

func TestCheckout_PromptsForVersionWhenMissing(t *testing.T) {
	dir := initLinkedDir(t, "cat-1")

	fc := &fakeClient{
		versions: []filesapi.CatalogVersion{{ID: verA}},
		content:  map[string][]byte{"a.txt": []byte("a")},
	}

	deps := fakeDeps(draftArtifact("art-abc-123"), fc)

	var promptedLabel string

	deps.PromptVersion = func(label string) (string, error) {
		promptedLabel = label

		return verA, nil
	}

	cmd, buf := newTestCmd(t, dir, deps, nil)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Code version ID", promptedLabel)
	assert.Contains(t, buf.String(), "dr artifact code versions")
	assert.DirExists(t, wapi.CheckoutDir(dir, verA))
}
