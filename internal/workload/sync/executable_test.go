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
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func skipOnWindows(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable bit")
	}
}

func TestClassifyExecutable(t *testing.T) {
	yes, no := boolPtr(true), boolPtr(false)

	cases := []struct {
		name                string
		base, local, remote *bool
		want                Classification
	}{
		{"unchanged", no, no, no, ClsUnchanged},
		{"local chmod +x", no, yes, no, ClsLocalModified},
		{"local chmod -x", yes, no, yes, ClsLocalModified},
		{"remote flag set", no, no, yes, ClsRemoteModified},
		{"remote flag cleared", yes, yes, no, ClsRemoteModified},
		{"both moved the same way", no, yes, yes, ClsUnchanged},
		{"old manifest entry", nil, yes, no, ClsUnchanged},
		{"old server", no, no, nil, ClsUnchanged},
		{"old server with a local chmod", no, yes, nil, ClsLocalModified},
		{"windows host", yes, nil, yes, ClsUnchanged},
		{"windows host with a remote change", no, nil, yes, ClsRemoteModified},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyExecutable(tc.base, tc.local, tc.remote))
		})
	}
}

func TestDiff_ExecutableOnlyChange(t *testing.T) {
	entry := func(exec *bool) FileEntry { return FileEntry{Hash: "h", Size: 1, Executable: exec} }

	t.Run("local chmod uploads", func(t *testing.T) {
		plan := Diff(
			BaseManifest{"run.sh": entry(boolPtr(false))},
			LocalManifest{"run.sh": entry(boolPtr(true))},
			RemoteManifest{"run.sh": entry(boolPtr(false))},
		)

		require.Len(t, plan.Uploads, 1)
		assert.Equal(t, ClsLocalModified, plan.Uploads[0].Classification)
		assert.True(t, plan.Uploads[0].ExecOnly())
	})

	t.Run("remote flag change downloads", func(t *testing.T) {
		plan := Diff(
			BaseManifest{"run.sh": entry(boolPtr(false))},
			LocalManifest{"run.sh": entry(boolPtr(false))},
			RemoteManifest{"run.sh": entry(boolPtr(true))},
		)

		require.Len(t, plan.Downloads, 1)
		assert.Equal(t, ClsRemoteModified, plan.Downloads[0].Classification)
		assert.Equal(t, boolPtr(true), plan.Downloads[0].RemoteExec)
		assert.Empty(t, plan.OverwrittenLocalPaths(), "same bytes locally: nothing to keep as a .LOCAL copy")
	})

	t.Run("unknown is not drift", func(t *testing.T) {
		for name, sides := range map[string][3]*bool{
			"old manifest": {nil, boolPtr(true), boolPtr(false)},
			"old server":   {boolPtr(true), boolPtr(true), nil},
			"windows host": {boolPtr(true), nil, boolPtr(true)},
		} {
			plan := Diff(
				BaseManifest{"run.sh": entry(sides[0])},
				LocalManifest{"run.sh": entry(sides[1])},
				RemoteManifest{"run.sh": entry(sides[2])},
			)

			assert.True(t, plan.IsEmpty(), name)
		}
	})

	t.Run("converged content still carries a local chmod", func(t *testing.T) {
		plan := Diff(
			BaseManifest{"run.sh": {Hash: "old", Size: 1, Executable: boolPtr(false)}},
			LocalManifest{"run.sh": {Hash: "new", Size: 1, Executable: boolPtr(true)}},
			RemoteManifest{"run.sh": {Hash: "new", Size: 1, Executable: boolPtr(false)}},
		)

		require.Len(t, plan.Uploads, 1)
	})
}

func TestUploadExecutable(t *testing.T) {
	assert.Equal(t, boolPtr(true), uploadExecutable(boolPtr(true), boolPtr(false)), "the local bit wins")
	assert.Equal(t, boolPtr(false), uploadExecutable(boolPtr(false), boolPtr(true)))
	assert.Equal(t, boolPtr(true), uploadExecutable(nil, boolPtr(true)), "Windows keeps the remote bit")
	assert.Nil(t, uploadExecutable(nil, nil))
}

func TestExecutableNotice(t *testing.T) {
	assert.Empty(t, executableNotice(nil))
	assert.Empty(t, executableNotice(map[string]FileEntry{"a.py": {Executable: boolPtr(false)}, "b.py": {}}))

	msg := executableNotice(map[string]FileEntry{
		"run.sh":  {Executable: boolPtr(true)},
		"app.py":  {Executable: boolPtr(false)},
		"bin/a":   {Executable: boolPtr(true)},
		"bin/b":   {Executable: boolPtr(true)},
		"bin/c":   {Executable: boolPtr(true)},
		"bin/d":   {Executable: boolPtr(true)},
		"bin/e.x": {Executable: boolPtr(true)},
	})

	assert.Contains(t, msg, "COPY --chmod=755")
	assert.Contains(t, msg, "bin/a, bin/b, bin/c, bin/d, bin/e.x and 1 more")
	assert.NotContains(t, msg, "app.py")
	assert.NotContains(t, msg, "\n")
}

func TestBuildZip_EntryModeCarriesExecutableBit(t *testing.T) {
	skipOnWindows(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("x"), 0o644))

	zipPath, sent, err := buildZip(dir, []FileAction{{Path: "run.sh"}, {Path: "app.py"}})
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.Remove(zipPath) })

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)

	t.Cleanup(func() { _ = r.Close() })

	modes := map[string]os.FileMode{}
	for _, f := range r.File {
		modes[f.Name] = f.Mode().Perm()
	}

	assert.Equal(t, os.FileMode(0o755), modes["run.sh"])
	assert.Equal(t, os.FileMode(0o644), modes["app.py"])
	assert.Equal(t, boolPtr(true), sent["run.sh"].Executable)
	assert.Equal(t, boolPtr(false), sent["app.py"].Executable)
}

func TestUploadOneToStage_SendsExecutableBit(t *testing.T) {
	skipOnWindows(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("x"), 0o644))

	fake := &fakeFilesClient{}
	e := &Engine{projectDir: dir, files: fake}

	for _, path := range []string{"run.sh", "app.py"} {
		entry, err := uploadOneToStage(e, "cid", "st", FileAction{Path: path})
		require.NoError(t, err)
		assert.Equal(t, path == "run.sh", isSet(entry.Executable), path)
	}

	assert.True(t, fake.stagedExec["run.sh"])
	assert.False(t, fake.stagedExec["app.py"])
}

func TestDownloadOne_AppliesRemoteExecutableBit(t *testing.T) {
	skipOnWindows(t)

	dir := t.TempDir()
	client := &downloadFake{content: map[string]string{"run.sh": "#!/bin/sh\n", "app.py": "x"}}

	require.NoError(t, DownloadOne(client, dir, "cid", "v", FileAction{Path: "run.sh", RemoteExec: boolPtr(true)}))
	assertPerm(t, filepath.Join(dir, "run.sh"), 0o755)

	// os.Create keeps an existing file's mode, so a cleared flag has to clear it.
	require.NoError(t, DownloadOne(client, dir, "cid", "v", FileAction{Path: "run.sh", RemoteExec: boolPtr(false)}))
	assertPerm(t, filepath.Join(dir, "run.sh"), 0o644)

	require.NoError(t, DownloadOne(client, dir, "cid", "v", FileAction{Path: "app.py"}))
	assertPerm(t, filepath.Join(dir, "app.py"), 0o644)
}

func TestRollback_KeepsExecutableBit(t *testing.T) {
	skipOnWindows(t)

	dir := setupProject(t)
	writeProjectFile(t, dir, "run.sh", "#!/bin/sh\n")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))

	r, err := NewRollback(dir)
	require.NoError(t, err)
	require.NoError(t, r.Backup("run.sh"))

	writeProjectFile(t, dir, "run.sh", "BROKEN")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o644))

	require.NoError(t, r.Restore())
	assertPerm(t, filepath.Join(dir, "run.sh"), 0o755)
}

// execProject is a synced project whose base records run.sh as not executable,
// as a sync by this CLI would.
func execProject(t *testing.T, catalogID, versionID string) string {
	t.Helper()

	dir := syncedProject(t, map[string]string{"run.sh": "#!/bin/sh\n"}, catalogID, versionID)

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	for path, meta := range m.Files {
		meta.Executable = boolPtr(false)
		m.Files[path] = meta
	}

	require.NoError(t, wapi.SaveManifest(dir, m))

	return dir
}

func execEngine(t *testing.T, dir string, fake *fakeFilesClient, artifactVersion string) *Engine {
	t.Helper()

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, fake.catalogID, artifactVersion), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e
}

func TestEngine_ChmodOnlyUploadsNewVersion(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))

	fake := &fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v2"}

	result, err := execEngine(t, dir, fake, "v1").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.UploadedCount)
	assert.Equal(t, "v2", result.NewVersion, "a mode-only change is a new version like any other")

	assert.Equal(t, boolPtr(true), fake.versions["v2"]["run.sh"].Executable)

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(true), m.Files["run.sh"].Executable)

	// The recorded bit matches the disk, so the next plan is empty.
	plan, err := execEngine(t, dir, fake, "v2").Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty())
}

func TestEngine_RemoteExecutableFlagAppliedLocally(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	remote := map[string]filesapi.FileMeta{}
	for path, meta := range m.Files {
		remote[path] = filesapi.FileMeta{Hash: meta.Hash, Size: meta.Size, Executable: boolPtr(path == "run.sh")}
	}

	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v3"}).
		withVersion("cid", "v2", remote).
		withDownloadable("v2", map[string][]byte{"run.sh": []byte("#!/bin/sh\n")})

	result, err := execEngine(t, dir, fake, "v2").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.DownloadedCount)
	assert.Empty(t, result.ConflictCopies, "no .LOCAL copy for a bit flip")

	assertPerm(t, filepath.Join(dir, "run.sh"), 0o755)
	assertPerm(t, filepath.Join(dir, ignore.FileName), 0o644)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), path)
}
