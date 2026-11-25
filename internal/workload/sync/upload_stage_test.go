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
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStageUploadHashesStreamedBytes verifies that the per-path hash in
// UploadOutcome.Sent is computed from the bytes the server actually received,
// not from a re-hash of the file on disk. The fake records the bytes it
// received, so the assertion is "the recorded hash matches the received
// bytes' SHA-256" rather than "it matches the disk file.".
func TestStageUploadHashesStreamedBytes(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('orig')\n",
	}, catalogID, versionID)

	// Introduce a pending change so Plan sees an upload.
	modifyFile(t, dir, "app.py", "print('changed')\n")

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-new",
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Uploads, 1)

	result, err := e.Execute(plan)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The outcome must carry a Sent entry for app.py.
	require.NotNil(t, e.uploadOutcome, "Phase 5 must store the upload outcome on the engine")

	sent, ok := e.uploadOutcome.Sent["app.py"]
	require.True(t, ok, "Sent must have an entry for app.py")

	// The streamed hash must equal the SHA-256 of the bytes the fake
	// received, proving the hash is of the streamed bytes, not a re-hash
	// of the disk file.
	receivedBytes, ok := fake.uploadedFiles["app.py"]
	require.True(t, ok, "fake must have recorded the uploaded bytes")
	assert.Equal(t, sha256Hex(receivedBytes), sent.Hash,
		"Sent hash must match the SHA-256 of the bytes the server received")
	assert.Equal(t, int64(len(receivedBytes)), sent.Size,
		"Sent size must match the byte count the server received")
}

// TestStageUploadContentLengthFromOpenHandle verifies that content-length is
// derived from Stat() on the already-open handle, not from the Phase-2 planned
// size. A file that changes size between Plan and Execute must have the
// streamed size match the new size, not the planned size. This is only possible
// if the size comes from the open handle at upload time.
func TestStageUploadContentLengthFromOpenHandle(t *testing.T) {
	const (
		catalogID = "cid-synced"
		versionID = "ver-synced"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('orig')\n",
	}, catalogID, versionID)

	// Introduce a pending change so Plan sees an upload.
	modifyFile(t, dir, "app.py", "print('ho')\n") // 11 bytes

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-new",
	}

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
			PatchFn: func(_, _, _ string) error { return nil },
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Uploads, 1)

	plannedSize := plan.Uploads[0].LocalSize

	// Between Plan and Execute, grow the file. The streamed size must
	// match the new size, not the Phase-2 planned size.
	newContent := "print('hello world')\n" // 19 bytes
	modifyFile(t, dir, "app.py", newContent)

	require.NotEqual(t, plannedSize, int64(len(newContent)),
		"planned and streamed sizes must differ for the test to be meaningful")

	result, err := e.Execute(plan)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotNil(t, e.uploadOutcome)

	sent, ok := e.uploadOutcome.Sent["app.py"]
	require.True(t, ok)

	// The streamed size must match the bytes at Execute time, not the
	// Phase-2 planned size. This proves content-length came from the open
	// handle's Stat, not from fa.LocalSize.
	assert.Equal(t, int64(len(newContent)), sent.Size,
		"streamed size must match the file at Execute time, not the Phase-2 planned size")
	assert.NotEqual(t, plannedSize, sent.Size,
		"streamed size must differ from the Phase-2 planned size")

	// The hash must also match the new content.
	assert.Equal(t, sha256Hex([]byte(newContent)), sent.Hash,
		"streamed hash must match the content at Execute time")

	// The fake must have received exactly the new bytes.
	receivedBytes := fake.uploadedFiles["app.py"]
	assert.Equal(t, []byte(newContent), receivedBytes,
		"server must have received the bytes present at Execute time")
}
