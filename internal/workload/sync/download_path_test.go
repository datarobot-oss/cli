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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the download half of Execute through the fake's
// server model: a remote-modified file is pulled to disk, the bytes that
// land are asserted byte-for-byte and by checksum (not merely "no error"),
// and the two download fault hooks prove the failure paths trip the
// rollback instead of silently leaving a bad file behind.
//
// The download-plan shape recorded here is also the scaffolding later
// verify-milestone tests build on: a downloads-only plan whose rows carry
// the server's advertised hashes.

// downloadScenario builds a synced project whose disk is untouched while
// the server holds a newer version of app.py, so Plan() produces exactly
// one download row for it. The remote version is seeded with recorded
// content (not just hashes), which is what makes it downloadable through
// the fake. The .drignore bytes are seeded as-is so the ignore file is
// unchanged on both sides and does not add plan rows.
type downloadScenario struct {
	dir           string
	fake          *fakeFilesClient
	remoteContent string
	remoteHash    string
}

func newDownloadScenario(t *testing.T, mutate func(*fakeFilesClient) *fakeFilesClient) downloadScenario {
	t.Helper()

	const (
		catalogID   = "cid-dl"
		versionID   = "ver-dl-synced"
		remoteVerID = "ver-dl-remote"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	remoteContent := "print('remote-newer')\n"

	// The server holds a newer app.py than both the manifest and the disk;
	// the artifact's codeRef points at that version so the engine sees drift
	// and fetches the real remote listing.
	drignoreBytes, err := os.ReadFile(filepath.Join(dir, ignore.FileName))
	require.NoError(t, err)

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-dl",
		versionID: "ver-dl-next",
	}).withVersionContent(catalogID, remoteVerID, map[string][]byte{
		"app.py":        []byte(remoteContent),
		ignore.FileName: drignoreBytes,
	})

	if mutate != nil {
		fake = mutate(fake)
	}

	return downloadScenario{
		dir:           dir,
		fake:          fake,
		remoteContent: remoteContent,
		remoteHash:    sha256Hex([]byte(remoteContent)),
	}
}

func (s downloadScenario) engine(t *testing.T) *Engine {
	t.Helper()

	e, err := newWithDeps(s.dir, Options{Yes: true}, Deps{
		Files: s.fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "cid-dl", "ver-dl-remote"), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e
}

// TestExecute_RemoteModifiedDownloadLandsOnDisk is the fault-off control
// for the download path and the assertion the fake previously could not
// support: the download writes the server's exact bytes to disk, those
// bytes hash to the advertised checksum, and the manifest records that
// hash — "the right bytes were written", not merely "no error returned".
func TestExecute_RemoteModifiedDownloadLandsOnDisk(t *testing.T) {
	s := newDownloadScenario(t, nil)
	e := s.engine(t)

	plan, err := e.Plan()
	require.NoError(t, err)

	// The plan must be exactly one download row carrying the server's
	// advertised hash and size — the structure later downloads-only
	// coverage relies on.
	require.Len(t, plan.Downloads, 1)

	dl := plan.Downloads[0]
	assert.Equal(t, "app.py", dl.Path)
	assert.Equal(t, ActDownloadModify, dl.Action)
	assert.Equal(t, s.remoteHash, dl.RemoteHash, "plan must carry the advertised checksum")
	assert.Equal(t, int64(len(s.remoteContent)), dl.RemoteSize)
	assert.Empty(t, plan.Uploads)
	assert.Empty(t, plan.Deletes)
	assert.Empty(t, plan.Conflicts)

	result, err := e.Execute(plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.DownloadedCount)

	// The remote bytes must be on disk, byte for byte, and hash to the
	// advertised checksum.
	onDisk, err := os.ReadFile(filepath.Join(s.dir, "app.py"))
	require.NoError(t, err)

	assert.Equal(t, s.remoteContent, string(onDisk), "downloaded file must hold the server's exact bytes")
	assert.Equal(t, s.remoteHash, sha256Hex(onDisk), "downloaded bytes must hash to the advertised checksum")

	// The new BASE must describe what the server holds for the downloaded
	// path, which is only truthful if the write above really happened.
	manifest, err := wapi.LoadManifest(s.dir)
	require.NoError(t, err)
	assert.Equal(t, s.remoteHash, manifest.Files["app.py"].Hash, "manifest must record the downloaded file's remote hash")

	assert.Equal(t, 1, s.fake.DownloadFileCalls(), "exactly one download must have been served")
}

// TestExecute_FailedDownloadRollsBack proves a download that errors aborts
// the sync: Execute fails naming the path, and the rollback restores the
// original local file, so a failed download never leaves the tree half
// written.
func TestExecute_FailedDownloadRollsBack(t *testing.T) {
	s := newDownloadScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
		return f.withFailDownload("app.py")
	})
	e := s.engine(t)

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Downloads, 1)

	_, err = e.Execute(plan)
	require.Error(t, err, "a failed download must fail the sync")
	assert.Contains(t, err.Error(), "app.py", "the error must name the failed path")
	assert.Contains(t, err.Error(), "injected failure", "the fake's fault must be visible in the chain")

	// The rollback must have restored the pre-sync local content: the
	// remote bytes never landed despite the file having been opened for
	// writing mid-download.
	onDisk, readErr := os.ReadFile(filepath.Join(s.dir, "app.py"))
	require.NoError(t, readErr)
	assert.Equal(t, "print('hi')\n", string(onDisk), "failed download must leave the original local file in place")

	assert.Equal(t, 1, s.fake.DownloadFileCalls(), "the failed download still counts as a call")
}

// TestExecute_CorruptDownloadChecksumMismatchRollsBack proves the
// post-download checksum verification has teeth: the fake serves bytes
// whose hash differs from the advertised checksum (same length, so the
// size check passes), and the client must catch the mismatch, fail the
// sync, and restore the original file rather than keep corrupt bytes.
func TestExecute_CorruptDownloadChecksumMismatchRollsBack(t *testing.T) {
	s := newDownloadScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
		return f.withCorruptDownload("app.py")
	})
	e := s.engine(t)

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Downloads, 1)

	_, err = e.Execute(plan)
	require.Error(t, err, "a checksum mismatch must fail the sync")
	assert.Contains(t, err.Error(), "checksum mismatch", "the checksum check must be what catches the corruption")
	assert.Contains(t, err.Error(), "app.py", "the error must name the corrupted path")

	// The corrupt bytes were written and then removed by the download's
	// own cleanup; the rollback must have brought the original file back.
	onDisk, readErr := os.ReadFile(filepath.Join(s.dir, "app.py"))
	require.NoError(t, readErr)
	assert.Equal(t, "print('hi')\n", string(onDisk), "corrupt bytes must not survive a failed download")

	assert.Equal(t, 1, s.fake.DownloadFileCalls())
}
