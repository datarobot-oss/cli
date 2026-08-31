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
	"bytes"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readZipEntries opens a zip archive at zipPath, extracts every entry, and
// returns a map of archive-path → uncompressed content bytes. This is the
// genuine read-back assertion: we open the archive that buildZip produced
// and hash the extracted entries, rather than re-hashing the source files.
// The assertion is "the archive contains what we claimed," rather than a
// bare claim that something was hashed.
func readZipEntries(t *testing.T, zipPath string) map[string][]byte {
	t.Helper()

	data, err := os.ReadFile(zipPath)
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	entries := make(map[string][]byte)

	for _, zf := range zr.File {
		rc, err := zf.Open()
		require.NoError(t, err)

		content, err := io.ReadAll(rc)

		_ = rc.Close()

		require.NoError(t, err)

		entries[zf.Name] = content
	}

	return entries
}

// fileActionsFrom builds a sorted slice of FileActions from a map of
// relative paths to content. LocalHash and LocalSize are set from the
// content so the actions are realistic, but addToZip and uploadOneToStage
// do not read them — they hash the bytes that actually enter the archive.
func fileActionsFrom(files map[string]string) []FileAction {
	actions := make([]FileAction, 0, len(files))

	for path, content := range files {
		actions = append(actions, FileAction{
			Path:      path,
			LocalHash: sha256Hex([]byte(content)),
			LocalSize: int64(len(content)),
		})
	}

	sort.Slice(actions, func(i, j int) bool { return actions[i].Path < actions[j].Path })

	return actions
}

// TestZipBuild_HashesStreamedBytes verifies that buildZip records per-path
// Sent hashes equal to the SHA-256 of each file's content, and that the
// bytes read back out of the produced archive hash to the same values. This
// is the baseline correctness test for the zip path's hash-while-streaming
// behaviour: a normal multi-file archive with no mid-build changes.
//
// Read-back approach: we call buildZip (the production function), open the
// archive it returns the path to, extract each entry, and hash the extracted
// bytes. This proves the archive contains what the Sent map claims, rather
// than just that we hashed the source file.
// Fulfills VAL-UPLOAD-009 (sub-condition e: zip path produces hashes matching
// shasum of disk content).
func TestZipBuild_HashesStreamedBytes(t *testing.T) {
	files := map[string]string{
		"app.py":          "print('hello world')\n",
		"utils/helper.py": "def help(): pass\n",
		"config.yaml":     "key: value\n",
		"README.md":       "# Project\n\nA test project.\n",
	}

	dir := initProject(t, files)

	actions := fileActionsFrom(files)

	zipPath, sent, err := buildZip(dir, actions)
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.Remove(zipPath) })

	// Read back the produced archive and extract entries.
	archived := readZipEntries(t, zipPath)

	// Assert each file's Sent hash equals SHA-256 of its content, and
	// the bytes read back from the archive hash to the same value.
	for path, content := range files {
		entry, ok := sent[path]
		require.True(t, ok, "Sent must have an entry for %s", path)

		expectedHash := sha256Hex([]byte(content))
		expectedSize := int64(len(content))

		assert.Equal(t, expectedHash, entry.Hash,
			"Sent hash for %s must equal SHA-256 of file content", path)
		assert.Equal(t, expectedSize, entry.Size,
			"Sent size for %s must equal file byte count", path)

		// Read-back: the archive must contain the same bytes, and the
		// extracted bytes must hash to the same value recorded in Sent.
		archivedContent, ok := archived[path]
		require.True(t, ok, "archive must contain entry for %s", path)

		assert.Equal(t, []byte(content), archivedContent,
			"archived bytes for %s must match file content", path)
		assert.Equal(t, expectedHash, sha256Hex(archivedContent),
			"SHA-256 of extracted bytes for %s must match Sent hash", path)
	}
}

// TestZipBuild_SameSizeContentChange verifies that a file rewritten to
// different bytes of the SAME size between plan and addToZip records a Sent
// hash equal to the archived bytes' hash, NOT the Phase-2 planned hash. This
// is the zip-path analogue of the silent-poison TOCTOU case: the same-size
// rewrite produces no error and the hash must describe what entered the
// archive, not what was on disk at plan time.
// Fulfills VAL-UPLOAD-010.
func TestZipBuild_SameSizeContentChange(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py": "print('orig')\n",
	})

	// Phase-2 "planned" content: what was on disk at plan time.
	planContent := "print('aaaa')\n" // 12 bytes

	modifyFile(t, dir, "app.py", planContent)

	plannedHash := sha256Hex([]byte(planContent))
	plannedSize := int64(len(planContent))

	// Between plan and zip-build, rewrite to different bytes of the
	// SAME size. This is the silent-poison window.
	buildContent := "print('bbbb')\n" // 12 bytes, different content

	modifyFile(t, dir, "app.py", buildContent)

	require.Equal(t, plannedSize, int64(len(buildContent)),
		"planned and build content must be the same size for this test")

	// The FileAction carries the Phase-2 planned hash and size. addToZip
	// must NOT use these — it must hash the bytes that enter the archive.
	actions := []FileAction{{
		Path:      "app.py",
		LocalHash: plannedHash,
		LocalSize: plannedSize,
	}}

	zipPath, sent, err := buildZip(dir, actions)
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.Remove(zipPath) })

	entry, ok := sent["app.py"]
	require.True(t, ok, "Sent must have an entry for app.py")

	buildHash := sha256Hex([]byte(buildContent))

	// The Sent hash must be the archived bytes' hash, not the Phase-2
	// planned hash.
	assert.Equal(t, buildHash, entry.Hash,
		"Sent hash must equal SHA-256 of the archived bytes, not the Phase-2 planned hash")
	assert.NotEqual(t, plannedHash, entry.Hash,
		"Sent hash must differ from the Phase-2 planned hash")

	// Read-back: confirm the archive contains the build-time bytes, and
	// the extracted bytes hash to the recorded Sent hash.
	archived := readZipEntries(t, zipPath)

	archivedContent := archived["app.py"]
	require.NotNil(t, archivedContent, "archive must contain app.py")

	assert.Equal(t, []byte(buildContent), archivedContent,
		"archive must contain the build-time bytes, not the plan-time bytes")
	assert.Equal(t, buildHash, sha256Hex(archivedContent),
		"SHA-256 of extracted bytes must match the recorded Sent hash")
	assert.NotEqual(t, plannedHash, sha256Hex(archivedContent),
		"extracted bytes must NOT hash to the Phase-2 planned hash")
}

// TestZipBuild_SizeChange verifies that a file that grows or shrinks between
// plan and addToZip records a Sent size equal to the byte count that actually
// entered the archive (from io.Copy's return), not fa.LocalSize (the Phase-2
// planned size). The hash and size must be self-consistent — both describe
// the same streamed bytes — and the upload must not fail.
// Fulfills VAL-UPLOAD-020.
func TestZipBuild_SizeChange(t *testing.T) {
	tests := []struct {
		name         string
		planContent  string
		buildContent string
	}{
		{
			name:         "grows",
			planContent:  "print('hi')\n",    // 11 bytes
			buildContent: "print('hello')\n", // 14 bytes
		},
		{
			name:         "shrinks",
			planContent:  "print('hello')\n", // 14 bytes
			buildContent: "print('hi')\n",    // 11 bytes
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := initProject(t, map[string]string{
				"app.py": "print('orig')\n",
			})

			// Phase-2 planned content.
			modifyFile(t, dir, "app.py", tc.planContent)

			plannedHash := sha256Hex([]byte(tc.planContent))
			plannedSize := int64(len(tc.planContent))

			// Between plan and zip-build, change the file size.
			modifyFile(t, dir, "app.py", tc.buildContent)

			buildHash := sha256Hex([]byte(tc.buildContent))
			buildSize := int64(len(tc.buildContent))

			require.NotEqual(t, plannedSize, buildSize,
				"planned and build sizes must differ for this test to be meaningful")

			actions := []FileAction{{
				Path:      "app.py",
				LocalHash: plannedHash,
				LocalSize: plannedSize,
			}}

			zipPath, sent, err := buildZip(dir, actions)
			require.NoError(t, err, "upload must not fail due to a size change")

			t.Cleanup(func() { _ = os.Remove(zipPath) })

			entry, ok := sent["app.py"]
			require.True(t, ok, "Sent must have an entry for app.py")

			// Sent size must equal the bytes that entered the archive,
			// not the Phase-2 planned size (fa.LocalSize).
			assert.Equal(t, buildSize, entry.Size,
				"Sent size must equal the bytes that entered the archive, not fa.LocalSize")
			assert.NotEqual(t, plannedSize, entry.Size,
				"Sent size must differ from the Phase-2 planned size")

			// Hash and size must be self-consistent: both describe the
			// same streamed bytes.
			assert.Equal(t, buildHash, entry.Hash,
				"Sent hash must equal SHA-256 of the build-time bytes")
			assert.Equal(t, buildSize, entry.Size,
				"Sent size must equal the build-time byte count")

			// Read-back: confirm the archive contains the build-time
			// bytes, and the extracted bytes match the recorded Sent.
			archived := readZipEntries(t, zipPath)

			archivedContent := archived["app.py"]
			require.NotNil(t, archivedContent, "archive must contain app.py")

			assert.Equal(t, []byte(tc.buildContent), archivedContent,
				"archive must contain the build-time bytes")
			assert.Equal(t, buildHash, sha256Hex(archivedContent),
				"SHA-256 of extracted bytes must match the recorded Sent hash")
			assert.Equal(t, buildSize, int64(len(archivedContent)),
				"extracted byte count must match the recorded Sent size")
		})
	}
}

// TestStageAndZipProduceIdenticalSent verifies that given the same project
// tree, the stage path and the zip path produce identical Sent maps (same
// hashes and sizes for every file). Since Phase 6 seeds the manifest from
// Sent, identical Sent maps mean identical manifests. Both uploaders are
// driven directly through their exported ApplyUploads methods against
// self-consistent fakes, so the comparison exercises the real production
// code paths on both sides.
// Fulfills VAL-UPLOAD-009 (sub-condition e: stage and zip paths both produce
// manifests where every hash matches shasum of disk content).
func TestStageAndZipProduceIdenticalSent(t *testing.T) {
	files := map[string]string{
		"app.py":          "print('hello world')\n",
		"utils/helper.py": "def help(): pass\n",
		"config.yaml":     "key: value\n",
		"README.md":       "# Project\n",
	}

	dir := initProject(t, files)

	actions := fileActionsFrom(files)

	const catalogID = "cid-equiv"

	cid := catalogID

	// Stage path: drive StageUploader.ApplyUploads directly. The engine
	// only needs projectDir, files, and config.CatalogID for the upload
	// code path — no phases run.
	stageFake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-1",
		versionID: "ver-stage",
	}

	stageEngine := &Engine{
		projectDir: dir,
		files:      stageFake,
		config:     wapi.Config{CatalogID: &cid},
	}

	stageOutcome, err := StageUploader{}.ApplyUploads(stageEngine, actions)
	require.NoError(t, err)

	// Zip path: drive ZipUploader.ApplyUploads directly with a separate
	// fake so the two do not share server state.
	zipFake := &fakeFilesClient{
		catalogID: catalogID,
		versionID: "ver-zip",
	}

	zipEngine := &Engine{
		projectDir: dir,
		files:      zipFake,
		config:     wapi.Config{CatalogID: &cid},
	}

	zipOutcome, err := ZipUploader{}.ApplyUploads(zipEngine, actions)
	require.NoError(t, err)

	// Both paths must produce Sent maps with the same number of entries.
	require.Len(t, stageOutcome.Sent, len(actions),
		"stage Sent must have one entry per file")
	require.Len(t, zipOutcome.Sent, len(actions),
		"zip Sent must have one entry per file")

	// Both must produce identical hashes and sizes for every file, and
	// both must match the SHA-256 of the file content on disk.
	for _, fa := range actions {
		stageEntry, ok := stageOutcome.Sent[fa.Path]
		require.True(t, ok, "stage Sent must have entry for %s", fa.Path)

		zipEntry, ok := zipOutcome.Sent[fa.Path]
		require.True(t, ok, "zip Sent must have entry for %s", fa.Path)

		assert.Equal(t, stageEntry.Hash, zipEntry.Hash,
			"stage and zip must produce the same hash for %s", fa.Path)
		assert.Equal(t, stageEntry.Size, zipEntry.Size,
			"stage and zip must produce the same size for %s", fa.Path)

		// Both must match the SHA-256 and byte count of the file content.
		expectedHash := sha256Hex([]byte(files[fa.Path]))
		expectedSize := int64(len(files[fa.Path]))

		assert.Equal(t, expectedHash, stageEntry.Hash,
			"stage hash for %s must equal SHA-256 of content", fa.Path)
		assert.Equal(t, expectedHash, zipEntry.Hash,
			"zip hash for %s must equal SHA-256 of content", fa.Path)
		assert.Equal(t, expectedSize, stageEntry.Size,
			"stage size for %s must equal byte count", fa.Path)
		assert.Equal(t, expectedSize, zipEntry.Size,
			"zip size for %s must equal byte count", fa.Path)
	}
}
