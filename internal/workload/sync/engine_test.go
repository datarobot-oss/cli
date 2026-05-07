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

package sync

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	stdsync "sync"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeArtifactStore is the in-memory artifactStore used by engine tests.
// Each test sets only the function fields it needs; the rest panic to
// surface unexpected interactions.
type fakeArtifactStore struct {
	GetFn   func(id string) (*workload.Artifact, error)
	PatchFn func(artifactID, catalogID, catalogVersionID string) error
}

func (f *fakeArtifactStore) Get(id string) (*workload.Artifact, error) {
	if f.GetFn == nil {
		return nil, errors.New("fakeArtifactStore.Get: no GetFn configured")
	}

	return f.GetFn(id)
}

func (f *fakeArtifactStore) PatchCodeRef(artifactID, catalogID, catalogVersionID string) error {
	if f.PatchFn == nil {
		return nil
	}

	return f.PatchFn(artifactID, catalogID, catalogVersionID)
}

// fakeFilesClient is the in-memory FilesAPI fake used by engine tests.
// Unexpected methods return errors so off-happy-path drift fails loudly.
type fakeFilesClient struct {
	allFiles      map[string]filesapi.FileMeta
	catalogID     string
	versionID     string
	stageID       string
	uploadedFiles map[string][]byte
	deletedPaths  []string
	mu            stdsync.Mutex
}

func (f *fakeFilesClient) CreateCatalog() (*filesapi.CatalogResp, error) {
	if f.catalogID == "" {
		return nil, errors.New("fakeFilesClient.CreateCatalog: no catalogID configured")
	}

	return &filesapi.CatalogResp{CatalogID: f.catalogID, CatalogVersionID: ""}, nil
}

func (f *fakeFilesClient) CreateStage(_ string) (*filesapi.StageResp, error) {
	if f.stageID == "" {
		return nil, errors.New("fakeFilesClient.CreateStage: no stageID configured")
	}

	return &filesapi.StageResp{CatalogID: f.catalogID, StageID: f.stageID}, nil
}

func (f *fakeFilesClient) UploadToStage(_, _, name string, _ int64, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.uploadedFiles == nil {
		f.uploadedFiles = map[string][]byte{}
	}

	f.uploadedFiles[name] = data

	return nil
}

func (f *fakeFilesClient) ApplyStage(_, _, _ string) (*filesapi.ApplyStageResp, error) {
	if f.versionID == "" {
		return nil, errors.New("fakeFilesClient.ApplyStage: no versionID configured")
	}

	return &filesapi.ApplyStageResp{
		CatalogID:        f.catalogID,
		CatalogVersionID: f.versionID,
		NumFiles:         len(f.uploadedFiles),
	}, nil
}

func (f *fakeFilesClient) UploadFromZipNew(_ string, _ int64, _ io.Reader) (*filesapi.FromFileResp, error) {
	return nil, errors.New("fakeFilesClient: UploadFromZipNew not expected")
}

func (f *fakeFilesClient) UploadFromZipExisting(_, _, _ string, _ int64, _ io.Reader) (*filesapi.FromFileResp, error) {
	return nil, errors.New("fakeFilesClient: UploadFromZipExisting not expected")
}

func (f *fakeFilesClient) PollStatus(_ string) (*filesapi.StatusResp, error) {
	return nil, errors.New("fakeFilesClient: PollStatus not expected")
}

func (f *fakeFilesClient) AllFiles(_, _ string) (map[string]filesapi.FileMeta, error) {
	return f.allFiles, nil
}

func (f *fakeFilesClient) DownloadFile(_, _, _ string, _ io.Writer) (string, int64, error) {
	return "", 0, errors.New("fakeFilesClient: DownloadFile not expected")
}

func (f *fakeFilesClient) DeleteFiles(_ string, paths []string) (*filesapi.DeleteFilesResp, error) {
	f.deletedPaths = append(f.deletedPaths, paths...)
	return &filesapi.DeleteFilesResp{}, nil
}

func (f *fakeFilesClient) ListVersions(_ string, _ int) ([]filesapi.CatalogVersion, error) {
	return nil, errors.New("fakeFilesClient: ListVersions not expected")
}

func initProject(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "art-abc-123"}))

	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	return dir
}

func draftArtifact(id, catalogID, versionID string) *workload.Artifact {
	a := &workload.Artifact{ID: id, Name: id, Status: "DRAFT"}

	if catalogID == "" {
		return a
	}

	a.Spec = workload.Spec{
		ContainerGroups: []workload.ContainerGroup{
			{Containers: []workload.Container{
				{CodeRef: &workload.CodeRef{Datarobot: &workload.DatarobotCodeRef{
					CatalogID:        catalogID,
					CatalogVersionID: versionID,
				}}},
			}},
		},
	}

	return a
}

func TestEngine_Plan_FirstSyncEmptyArtifact(t *testing.T) {
	dir := initProject(t, map[string]string{
		"agent.py":        "print('hi')\n",
		"utils/helper.py": "def help(): pass\n",
	})

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	uploadPaths := make([]string, 0, len(plan.Uploads))
	for _, fa := range plan.Uploads {
		uploadPaths = append(uploadPaths, fa.Path)
	}

	assert.ElementsMatch(t, []string{".wapiignore", "agent.py", "utils/helper.py"}, uploadPaths)
	assert.Empty(t, plan.Downloads)
	assert.Empty(t, plan.Deletes)
	assert.Empty(t, plan.Conflicts)
}

func TestEngine_Plan_FastPathUpToDate(t *testing.T) {
	// Fast path must skip AllFiles when artifact codeRef matches
	// lastSyncedVersionId (no drift).
	dir := initProject(t, map[string]string{"agent.py": "x"})

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	cid := "cid-1"
	ver := "ver-1"
	cfg.CatalogID = &cid
	cfg.LastSyncedVersionID = &ver

	require.NoError(t, wapi.SaveConfig(dir, cfg))

	manifest := wapi.Manifest{Version: wapi.ManifestVersion, Files: map[string]wapi.FileMeta{}}

	for _, rel := range []string{"agent.py", ".wapiignore"} {
		hash, size, err := hashLocal(t, dir, rel)
		require.NoError(t, err)

		manifest.Files[rel] = wapi.FileMeta{Hash: hash, Size: size}
	}

	require.NoError(t, wapi.SaveManifest(dir, manifest))

	calledAllFiles := false

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &trackingFilesClient{
			fakeFilesClient: fakeFilesClient{},
			allFilesCalled:  &calledAllFiles,
		},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, cid, ver), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty(), "plan should be empty when local == base == remote: %+v", plan)
	assert.False(t, calledAllFiles, "AllFiles must not be called when not drifted")
}

func TestEngine_Plan_LockedArtifactRejected(t *testing.T) {
	dir := initProject(t, nil)

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return &workload.Artifact{ID: id, Status: "LOCKED"}, nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	_, err = e.Plan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked")
}

func TestEngine_Plan_NotLinked(t *testing.T) {
	dir := t.TempDir()

	e, err := New(dir, Options{})
	require.NoError(t, err)

	_, err = e.Plan()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not linked")
}

func TestEngine_Run_FirstSyncStagePath(t *testing.T) {
	dir := initProject(t, map[string]string{
		"agent.py":        "print('hi')\n",
		"utils/helper.py": "def help(): pass\n",
	})

	fake := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	var patchedArtifactID, patchedCatalogID, patchedVersionID string

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
			PatchFn: func(artifactID, catalogID, catalogVersionID string) error {
				patchedArtifactID = artifactID
				patchedCatalogID = catalogID
				patchedVersionID = catalogVersionID

				return nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	result, err := e.Run()
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "ver-1", result.NewVersion)
	assert.Equal(t, 3, result.UploadedCount, "expect agent.py + utils/helper.py + .wapiignore")

	assert.Len(t, fake.uploadedFiles, 3)
	assert.Equal(t, []byte("print('hi')\n"), fake.uploadedFiles["agent.py"])

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg.CatalogID)
	assert.Equal(t, "cid-new", *cfg.CatalogID)
	require.NotNil(t, cfg.LastSyncedVersionID)
	assert.Equal(t, "ver-1", *cfg.LastSyncedVersionID)

	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Len(t, manifest.Files, 3)

	// First-sync stage path must PATCH the artifact's codeRef so the
	// workload picks up the new catalog version. Without it, every
	// successive sync would re-detect drift.
	assert.NotEmpty(t, patchedArtifactID, "PatchArtifactCodeRef must be called after upload")
	assert.Equal(t, "cid-new", patchedCatalogID)
	assert.Equal(t, "ver-1", patchedVersionID)
}

type trackingFilesClient struct {
	fakeFilesClient
	allFilesCalled *bool
}

func (t *trackingFilesClient) AllFiles(cid, vid string) (map[string]filesapi.FileMeta, error) {
	*t.allFilesCalled = true
	return t.fakeFilesClient.AllFiles(cid, vid)
}

func hashLocal(t *testing.T, dir, rel string) (string, int64, error) {
	t.Helper()

	abs := filepath.Join(dir, filepath.FromSlash(rel))

	return fileops.HashFile(abs)
}
