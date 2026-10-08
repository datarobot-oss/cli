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
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chunkBoundarySize is the default io.Copy buffer size (32 KiB). A file whose
// size is an exact multiple of this value exercises the chunk-boundary edge
// case: the last io.Copy chunk is exactly one buffer, not a partial.
const chunkBoundarySize = 32 * 1024

// TestConcurrentUpload_RaceFree_NoDroppedResults drives the stage uploader
// with 4-way concurrency (UploadConcurrency) over 13 files of mixed sizes —
// including 0-byte, small, and chunk-boundary sizes — and asserts:
//   - No data race (automatic under `go test -race`).
//   - Exactly one Sent entry per planned upload (no dropped result from a
//     blocked channel send or an early worker return).
//   - No file is uploaded twice (UploadToStageCalls == len(files)).
//   - Every Sent hash equals the SHA-256 of the file content on disk.
//
// The result channel is buffered to len(files) so a send never blocks, and
// the orchestrator drains it after wg.Wait. A bug that closed resCh early,
// used an unbuffered channel, or returned before sending would drop a result
// and the Sent-count assertion would catch it.
// Fulfills VAL-UPLOAD-012.
func TestConcurrentUpload_RaceFree_NoDroppedResults(t *testing.T) {
	// 13 files of mixed sizes: 0-byte, small, chunk-boundary, and larger.
	fileSpecs := []struct {
		path string
		size int
	}{
		{"f00.dat", 0},
		{"f01.dat", 0},
		{"f02.dat", 1},
		{"f03.dat", 10},
		{"f04.dat", 100},
		{"f05.dat", 500},
		{"f06.dat", 1000},
		{"f07.dat", chunkBoundarySize},
		{"f08.dat", chunkBoundarySize},
		{"f09.dat", chunkBoundarySize + 1},
		{"f10.dat", chunkBoundarySize * 2},
		{"f11.dat", 50000},
		{"f12.dat", 0},
	}

	dir := initProject(t, nil)

	// Write the files and build FileActions with known content.
	actions := make([]FileAction, 0, len(fileSpecs))

	expectedHashes := make(map[string]string, len(fileSpecs))

	for _, spec := range fileSpecs {
		content := generateContent(spec.path, spec.size)

		abs := filepath.Join(dir, spec.path)
		require.NoError(t, os.WriteFile(abs, content, 0o644))

		actions = append(actions, FileAction{
			Path:      spec.path,
			LocalHash: sha256Hex(content),
			LocalSize: int64(len(content)),
		})

		expectedHashes[spec.path] = sha256Hex(content)
	}

	const catalogID = "cid-conc"

	cid := catalogID

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-conc",
	}

	engine := &Engine{
		projectDir: dir,
		files:      fake,
		config:     wapi.Config{CatalogID: &cid},
	}

	outcome, err := StageUploader{}.ApplyUploads(engine, actions)

	require.NoError(t, err, "all uploads must succeed with no fault injection")

	// Exactly one Sent entry per planned upload — no dropped result.
	require.Len(t, outcome.Sent, len(fileSpecs),
		"Sent must have exactly one entry per planned file (no dropped results)")

	// No file uploaded twice: UploadToStageCalls must equal the file count.
	assert.Equal(t, len(fileSpecs), fake.UploadToStageCalls(),
		"UploadToStage must be called exactly once per file (no duplicate uploads)")

	// ApplyStage called exactly once (the stage path applies in a single call).
	assert.Equal(t, 1, fake.ApplyStageCalls(),
		"ApplyStage must be called exactly once")

	// Every Sent entry must have the correct hash and size.
	for _, fa := range actions {
		entry, ok := outcome.Sent[fa.Path]
		require.True(t, ok, "Sent must have an entry for %s", fa.Path)

		assert.Equal(t, expectedHashes[fa.Path], entry.Hash,
			"Sent hash for %s must equal SHA-256 of file content", fa.Path)
		assert.Equal(t, fa.LocalSize, entry.Size,
			"Sent size for %s must equal file byte count", fa.Path)
	}

	// The fake's server state must reflect every uploaded file with the
	// correct checksum. This is the self-consistency check: AllFiles returns
	// exactly what was uploaded.
	all, err := fake.AllFiles(catalogID, "ver-conc")
	require.NoError(t, err)

	assert.Len(t, all, len(fileSpecs),
		"server must hold exactly one entry per uploaded file")

	for _, fa := range actions {
		fm, ok := all[fa.Path]
		require.True(t, ok, "server must have %s", fa.Path)

		assert.Equal(t, expectedHashes[fa.Path], fm.Hash,
			"server checksum for %s must equal SHA-256 of content", fa.Path)
		assert.Equal(t, fa.LocalSize, fm.Size,
			"server size for %s must equal file byte count", fa.Path)
	}
}

// generateContent produces deterministic content of the given size for a
// given path. The content is a repeating pattern of the path name so each
// file has distinct bytes (avoiding hash collisions) while being fully
// deterministic across runs.
func generateContent(path string, size int) []byte {
	if size == 0 {
		return []byte{}
	}

	pattern := path + ":"

	return []byte(strings.Repeat(pattern, size/len(pattern)+1))[:size]
}
