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
		{"old manifest entry, the old upload dropped the bit", nil, yes, no, ClsLocalModified},
		{"old manifest entry, the old download dropped the bit", nil, no, yes, ClsRemoteModified},
		{"old manifest entry, both agree", nil, yes, yes, ClsUnchanged},
		{"old manifest entry on windows", nil, nil, yes, ClsUnchanged},
		{"old manifest entry on an old server", nil, yes, nil, ClsUnchanged},
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
			"old manifest, old server": {nil, boolPtr(true), nil},
			"old server":               {boolPtr(true), boolPtr(true), nil},
			"windows host":             {boolPtr(true), nil, boolPtr(true)},
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

// The bit is merged apart from the bytes: whichever side moved it off BASE
// wins, so a chmod survives an edit to the same file on the other side.
func TestMergeExecutable(t *testing.T) {
	yes, no := boolPtr(true), boolPtr(false)

	cases := []struct {
		name                string
		base, local, remote *bool
		want                *bool
	}{
		{"nothing moved", no, no, no, no},
		{"local chmod +x, teammate edited", no, yes, no, yes},
		{"teammate chmod +x, local edit", no, no, yes, yes},
		{"local chmod -x", yes, no, yes, no},
		{"teammate chmod -x", yes, yes, no, no},
		{"windows keeps the remote's chmod", no, nil, yes, yes},
		{"windows keeps the base", yes, nil, yes, yes},
		{"old manifest, executable side wins", nil, no, yes, yes},
		{"old manifest, local known only", nil, no, nil, no},
		{"nothing known", nil, nil, nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mergeExecutable(tc.base, tc.local, tc.remote))
		})
	}
}

func TestExecutableNotice(t *testing.T) {
	assert.Empty(t, executableNotice(nil, nil, false))
	assert.Empty(t, executableNotice(map[string]FileEntry{"a.py": {Executable: boolPtr(false)}, "b.py": {}}, nil, false),
		"an unknown bit, as on a server that never took it, names nothing")

	// An edit to a script the catalog already holds as executable is not news.
	assert.Empty(t, executableNotice(
		map[string]FileEntry{"run.sh": {Executable: boolPtr(true)}},
		RemoteManifest{"run.sh": {Executable: boolPtr(true)}},
		false,
	))

	// A generated build has no Dockerfile to edit, so it gets no COPY advice.
	generated := executableNotice(map[string]FileEntry{"run.sh": {Executable: boolPtr(true)}}, nil, true)
	assert.Contains(t, generated, "run.sh")
	assert.NotContains(t, generated, "COPY")

	msg := executableNotice(map[string]FileEntry{
		"run.sh":  {Executable: boolPtr(true)},
		"app.py":  {Executable: boolPtr(false)},
		"bin/a":   {Executable: boolPtr(true)},
		"bin/b":   {Executable: boolPtr(true)},
		"bin/c":   {Executable: boolPtr(true)},
		"bin/d":   {Executable: boolPtr(true)},
		"bin/e.x": {Executable: boolPtr(true)},
	}, nil, false)

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

	zipPath, sent, err := buildZip(dir, []FileAction{{Path: "run.sh"}, {Path: "app.py"}}, true)
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

	// Nothing is registered as downloadable: the bytes already match, so the
	// bit is applied in place and a download attempt would fail the run.
	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v3"}).
		withVersion("cid", "v2", remote)

	result, err := execEngine(t, dir, fake, "v2").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.DownloadedCount)
	assert.Empty(t, result.ConflictCopies, "no .LOCAL copy for a bit flip")

	assertPerm(t, filepath.Join(dir, "run.sh"), 0o755)
	assertPerm(t, filepath.Join(dir, ignore.FileName), 0o644)
}

// oldCLIProject is a project an older CLI synced: run.sh is executable on
// disk, the manifest records no bits, and the catalog holds the dropped bit.
func oldCLIProject(t *testing.T) (string, *fakeFilesClient) {
	t.Helper()

	dir := syncedProject(t, map[string]string{"run.sh": "#!/bin/sh\n"}, "cid", "v1")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	remote := map[string]filesapi.FileMeta{}

	for path, meta := range m.Files {
		remote[path] = filesapi.FileMeta{Hash: meta.Hash, Size: meta.Size, Executable: boolPtr(false)}
		meta.Executable = nil
		m.Files[path] = meta
	}

	require.NoError(t, wapi.SaveManifest(dir, m))

	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v2"}).withVersion("cid", "v1", remote)

	return dir, fake
}

// The reported case: nothing was edited since an older CLI uploaded the
// executable without its bit. The fast path lists the catalog once, and the
// executable side wins, so the file goes up again with the bit.
func TestEngine_OldManifestUploadsTheDroppedBit(t *testing.T) {
	skipOnWindows(t)

	dir, fake := oldCLIProject(t)

	result, err := execEngine(t, dir, fake, "v1").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, fake.AllFilesCalls(), "the old manifest triggers one listing")
	assert.Equal(t, 1, result.UploadedCount)
	assert.Equal(t, boolPtr(true), fake.versions["v2"]["run.sh"].Executable)
}

// When the catalog already agrees, the plan is empty and never reaches the
// state phase, so the learned bits are written by the plan and the next sync
// takes the fast path again.
func TestEngine_OldManifestRecordsBitsOnce(t *testing.T) {
	skipOnWindows(t)

	dir, fake := oldCLIProject(t)
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o644))

	first := execEngine(t, dir, fake, "v1")

	plan, err := first.Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty())
	require.NoError(t, first.Close())

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, boolPtr(false), m.Files["run.sh"].Executable)

	_, err = execEngine(t, dir, fake, "v1").Plan()
	require.NoError(t, err)
	assert.Equal(t, 1, fake.AllFilesCalls(), "the second sync takes the fast path")
}

// A preview lists the catalog so it shows the fix the sync would make, and
// writes nothing.
func TestEngine_OldManifestDryRunWritesNothing(t *testing.T) {
	skipOnWindows(t)

	dir, fake := oldCLIProject(t)

	before, err := os.ReadFile(filepath.Join(wapi.Dir(dir), "manifest.json"))
	require.NoError(t, err)

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{GetFn: func(id string) (*workload.Artifact, error) {
			return draftArtifact(id, "cid", "v1"), nil
		}},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)
	assert.Len(t, plan.Uploads, 1, "the preview shows the upload the sync would make")

	after, err := os.ReadFile(filepath.Join(wapi.Dir(dir), "manifest.json"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	// Only the executable bits: the rest follow the host's umask.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want&0o111 != 0, info.Mode().Perm()&0o111 != 0, path)
}
