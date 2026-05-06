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

package versions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient is a minimal in-memory filesapi.Client so the cmd tests
// don't have to spin up an httptest server.
type fakeClient struct {
	versions   []filesapi.CatalogVersion
	listErr    error
	gotCatalog string
	gotLimit   int
	listCalls  int
}

func (f *fakeClient) ListVersions(catalogID string, limit int) ([]filesapi.CatalogVersion, error) {
	f.listCalls++
	f.gotCatalog = catalogID
	f.gotLimit = limit

	return f.versions, f.listErr
}

// Unused interface methods.
func (*fakeClient) CreateCatalog() (*filesapi.CatalogResp, error) { panic("unused") }

func (*fakeClient) CreateStage(string) (*filesapi.StageResp, error) {
	panic("unused")
}

func (*fakeClient) UploadToStage(string, string, string, int64, io.Reader) error {
	panic("unused")
}

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
func (*fakeClient) AllFiles(string, string) (map[string]filesapi.FileMeta, error) {
	panic("unused")
}

func (*fakeClient) DownloadFile(string, string, string, io.Writer) (string, int64, error) {
	panic("unused")
}

func (*fakeClient) DeleteFiles(string, []string) (*filesapi.DeleteFilesResp, error) {
	panic("unused")
}

// withSeams installs test doubles for getArtifactFn and newClientFn for
// the duration of t.
func withSeams(t *testing.T, art *workload.Artifact, fc *fakeClient) {
	t.Helper()

	origArt := getArtifactFn
	origCli := newClientFn

	getArtifactFn = func(_ string) (*workload.Artifact, error) { return art, nil }
	newClientFn = func() filesapi.Client { return fc }

	t.Cleanup(func() {
		getArtifactFn = origArt
		newClientFn = origCli
	})
}

func newTestCmd(t *testing.T, dir string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	var buf bytes.Buffer

	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	require.NoError(t, cmd.Flags().Set("dir", dir))

	return cmd, &buf
}

func draftArtifact(id, name, currentVersion string) *workload.Artifact {
	a := &workload.Artifact{ID: id, Name: name, Status: "draft"}

	if currentVersion == "" {
		return a
	}

	a.Spec = workload.Spec{
		ContainerGroups: []workload.ContainerGroup{{
			Containers: []workload.Container{{
				CodeRef: &workload.CodeRef{Datarobot: &workload.DatarobotCodeRef{
					CatalogID:        "cat-1",
					CatalogVersionID: currentVersion,
				}},
			}},
		}},
	}

	return a
}

func initLinkedDir(t *testing.T, catalogID, syncedVersion string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-abc-123"}))

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	cfg.CatalogID = &catalogID
	if syncedVersion != "" {
		cfg.LastSyncedVersionID = &syncedVersion
	}

	require.NoError(t, wapi.SaveConfig(dir, cfg))

	return dir
}

func TestVersions_NotLinked(t *testing.T) {
	dir := t.TempDir()

	cmd, _ := newTestCmd(t, dir)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

func TestVersions_NoCatalog(t *testing.T) {
	// init creates .wapi/ with no catalogId set.
	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-abc-123"}))

	cmd, _ := newTestCmd(t, dir)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has been synced")
}

func TestVersions_TextOutput(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "v2")

	withSeams(
		t,
		draftArtifact("art-abc-123", "my-agent", "v3aaaaaaaaaaaa"),
		&fakeClient{
			versions: []filesapi.CatalogVersion{
				{ID: "v3aaaaaaaaaaaa", CreatedAt: "2026-04-10T14:30:00Z", NumFiles: 47, TotalSize: 2412544},
				{ID: "v2bbbbbbbbbbbb", CreatedAt: "2026-04-10T10:15:00Z", NumFiles: 46, TotalSize: 2300000},
				{ID: "v1ccccccccccccc", CreatedAt: "2026-04-09T16:45:00Z", NumFiles: 45, TotalSize: 2100000},
			},
		},
	)

	cmd, buf := newTestCmd(t, dir)

	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "Artifact: my-agent (art-abc-123)")
	assert.Contains(t, out, "Status:   DRAFT")
	assert.Contains(t, out, "VERSION ID")
	assert.Contains(t, out, "* v3aaaaaa")
	assert.Contains(t, out, "v2bbbbbb")
	assert.Contains(t, out, "* = current")
}

func TestVersions_LimitFlagPropagates(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	fc := &fakeClient{}

	withSeams(
		t,
		draftArtifact("art-abc-123", "my-agent", ""),
		fc,
	)

	cmd, _ := newTestCmd(t, dir)
	require.NoError(t, cmd.Flags().Set("limit", "5"))

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "cat-1", fc.gotCatalog)
	assert.Equal(t, 5, fc.gotLimit)
}

func TestVersions_JSONOutput(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	withSeams(
		t,
		draftArtifact("art-abc-123", "my-agent", "v3aaaaaaaaaaaa"),
		&fakeClient{
			versions: []filesapi.CatalogVersion{
				{ID: "v3aaaaaaaaaaaa", CreatedAt: "2026-04-10T14:30:00Z", NumFiles: 1, TotalSize: 100},
				{ID: "v2bbbbbbbbbbbb", CreatedAt: "2026-04-10T10:15:00Z", NumFiles: 1, TotalSize: 50},
			},
		},
	)

	cmd, buf := newTestCmd(t, dir)
	require.NoError(t, cmd.Flags().Set("output-format", "json"))

	require.NoError(t, cmd.Execute())

	var got []jsonRow

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "v3aaaaaaaaaaaa", got[0].VersionID)
	assert.Equal(t, "v3aaaaaa", got[0].VersionShort)
	assert.True(t, got[0].IsCurrent)
	assert.False(t, got[1].IsCurrent)
}

func TestVersions_ListVersionsErrorPropagates(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	withSeams(
		t,
		draftArtifact("art-abc-123", "my-agent", ""),
		&fakeClient{listErr: errors.New("boom")},
	)

	cmd, _ := newTestCmd(t, dir)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list versions")
	assert.Contains(t, err.Error(), "boom")
}

func TestVersions_NegativeLimitRejected(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	cmd, _ := newTestCmd(t, dir)
	require.NoError(t, cmd.Flags().Set("limit", "-5"))

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --limit -5")
	assert.Contains(t, err.Error(), "0 = unlimited")
}

func TestVersions_ArtifactNotFoundSpecialized(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	origArt := getArtifactFn

	t.Cleanup(func() { getArtifactFn = origArt })

	getArtifactFn = func(_ string) (*workload.Artifact, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "test"}
	}

	cmd, _ := newTestCmd(t, dir)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, "artifact art-abc-123 not found", err.Error())
}

func TestVersions_CatalogNotFoundSpecialized(t *testing.T) {
	dir := initLinkedDir(t, "cat-1", "")

	withSeams(
		t,
		draftArtifact("art-abc-123", "my-agent", ""),
		&fakeClient{listErr: &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "test"}},
	)

	cmd, _ := newTestCmd(t, dir)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, "catalog cat-1 not found", err.Error())
}
