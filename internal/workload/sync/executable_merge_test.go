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
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remoteFromBase is the catalog at the synced version as the base describes
// it, with run.sh's bytes and bit overridden.
func remoteFromBase(t *testing.T, dir string, runSh []byte, runShExec bool) map[string]filesapi.FileMeta {
	t.Helper()

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	remote := map[string]filesapi.FileMeta{}

	for path, meta := range m.Files {
		remote[path] = filesapi.FileMeta{Hash: meta.Hash, Size: meta.Size, Executable: boolPtr(false)}
	}

	h := sha256.Sum256(runSh)
	remote["run.sh"] = filesapi.FileMeta{Hash: hex.EncodeToString(h[:]), Size: int64(len(runSh)), Executable: boolPtr(runShExec)}

	return remote
}

func assertExec(t *testing.T, path string, want bool) {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm()&0o111 != 0, path)
}

// I edit run.sh while a teammate makes it executable: my upload keeps their
// bit, my copy gets it too, and the next sync has nothing to do.
func TestEngine_LocalEditKeepsTeammatesChmod(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")
	runSh := filepath.Join(dir, "run.sh")

	original, err := os.ReadFile(runSh)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(runSh, []byte("#!/bin/sh\necho edited\n"), 0o644))

	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v3"}).
		withVersion("cid", "v2", remoteFromBase(t, dir, original, true))

	result, err := execEngine(t, dir, fake, "v2").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.UploadedCount)

	assert.Equal(t, boolPtr(true), fake.versions["v3"]["run.sh"].Executable, "the teammate's +x is not cleared")
	assertExec(t, runSh, true)

	plan, err := execEngine(t, dir, fake, "v3").Plan()
	require.NoError(t, err)
	assert.True(t, plan.IsEmpty(), "disk and base agree, so nothing reads as a chmod")
}

// A teammate edits run.sh while I make it executable: their bytes come down,
// my bit stays, and the next sync uploads it.
func TestEngine_RemoteEditKeepsLocalChmod(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")
	runSh := filepath.Join(dir, "run.sh")
	require.NoError(t, os.Chmod(runSh, 0o755))

	edited := []byte("#!/bin/sh\necho theirs\n")

	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v3"}).
		withVersion("cid", "v2", remoteFromBase(t, dir, edited, false))
	fake.versionContents = map[string]map[string][]byte{"v2": {"run.sh": edited}}

	result, err := execEngine(t, dir, fake, "v2").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.DownloadedCount)

	got, err := os.ReadFile(runSh)
	require.NoError(t, err)
	assert.Equal(t, edited, got)
	assertExec(t, runSh, true)

	plan, err := execEngine(t, dir, fake, "v2").Plan()
	require.NoError(t, err)
	require.Len(t, plan.Uploads, 1, "the kept chmod goes up next")
	assert.Equal(t, "run.sh", plan.Uploads[0].Path)
}

// Push-only sets aside the catalog's +x on a file an older CLI synced, and
// the plan is otherwise empty: the bit must not reach BASE, or the next plain
// sync reads the 0644 disk copy as a local chmod -x and clears the catalog.
func TestEngine_PushOnlyBackfillLeavesSkippedBitsUnknown(t *testing.T) {
	skipOnWindows(t)

	dir, fake := oldCLIProject(t)
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o644))

	for path, meta := range fake.versions["v1"] {
		if path == "run.sh" {
			meta.Executable = boolPtr(true)
			fake.versions["v1"][path] = meta
		}
	}

	e, err := newWithDeps(dir, Options{Yes: true, PushOnly: true}, Deps{
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
	require.Len(t, plan.Skipped, 1)

	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Nil(t, m.Files["run.sh"].Executable, "a set-aside file's bit is not recorded")
}

// A server older than API 2.49 never received the bit, so BASE records it as
// unknown rather than claiming what the catalog does not hold; nothing is
// warned about; and once a listing shows it reports no bits, the fast path
// stops listing it.
func TestEngine_OldServerRecordsNoBitAndStopsListing(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))

	// The catalog holds every synced file, and reports no bits.
	m, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	v1 := map[string]filesapi.FileMeta{}
	for path, meta := range m.Files {
		v1[path] = filesapi.FileMeta{Hash: meta.Hash, Size: meta.Size}
	}

	fake := (&fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v2", oldServer: true}).
		withVersion("cid", "v1", v1)

	result, err := execEngine(t, dir, fake, "v1").Run()
	require.NoError(t, err)
	assert.Equal(t, 1, result.UploadedCount)
	assert.Empty(t, result.ExecutableNotice, "the bit never reached the catalog")

	m, err = wapi.LoadManifest(dir)
	require.NoError(t, err)
	assert.Nil(t, m.Files["run.sh"].Executable)

	first := execEngine(t, dir, fake, "v2")
	_, err = first.Plan()
	require.NoError(t, err)
	require.NoError(t, first.Close())
	assert.Equal(t, 1, fake.AllFilesCalls(), "the unknown bit triggers one listing")

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)
	assert.True(t, cfg.ExecutableUnreported)

	_, err = execEngine(t, dir, fake, "v2").Plan()
	require.NoError(t, err)
	assert.Equal(t, 1, fake.AllFilesCalls(), "the next sync takes the fast path")
}

// The notice travels on the result, so up can print it after its progress
// display, and its advice fits a project that writes its own Dockerfile.
func TestEngine_ResultCarriesExecutableNotice(t *testing.T) {
	skipOnWindows(t)

	dir := execProject(t, "cid", "v1")
	require.NoError(t, os.Chmod(filepath.Join(dir, "run.sh"), 0o755))

	fake := &fakeFilesClient{catalogID: "cid", stageID: "st", versionID: "v2"}

	result, err := execEngine(t, dir, fake, "v1").Run()
	require.NoError(t, err)
	assert.Contains(t, result.ExecutableNotice, "run.sh")
	assert.Contains(t, result.ExecutableNotice, "COPY --chmod=755")
}

// A chmod-only download moves no bytes, so it does not count toward the
// space the downloads need.
func TestTotalDownloadBytes_SkipsExecOnly(t *testing.T) {
	plan := &SyncPlan{Downloads: []FileAction{
		{Path: "big.bin", LocalHash: "aa", RemoteHash: "aa", RemoteSize: 1 << 30, Action: ActDownloadModify},
		{Path: "new.txt", RemoteHash: "bb", RemoteSize: 10, Action: ActDownloadModify},
	}}

	assert.Equal(t, int64(10), plan.TotalDownloadBytes())
}
