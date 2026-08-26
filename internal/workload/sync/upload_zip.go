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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
)

// ZipUploader implements the async zip workflow: build a zip locally,
// POST it to FilesAPI, poll until terminal.
type ZipUploader struct{}

// ApplyUploads zips the files, POSTs, polls until done, and returns the
// per-path streamed hashes so Phase 6 can record what entered the archive.
func (ZipUploader) ApplyUploads(e *Engine, files []FileAction) (UploadOutcome, error) {
	zipPath, sent, err := buildZip(e.projectDir, files)
	if err != nil {
		return UploadOutcome{}, err
	}

	defer func() { _ = os.Remove(zipPath) }()

	zipFile, err := os.Open(zipPath)
	if err != nil {
		return UploadOutcome{}, fmt.Errorf("open built zip: %w", err)
	}

	defer func() { _ = zipFile.Close() }()

	stat, err := zipFile.Stat()
	if err != nil {
		return UploadOutcome{}, fmt.Errorf("stat built zip: %w", err)
	}

	resp, err := postZip(e, zipFile, stat.Size())
	if err != nil {
		return UploadOutcome{}, err
	}

	// Small archives complete inline (201, no statusId); larger ones
	// come back 202 with a statusId we then poll.
	if resp.StatusID != "" {
		if err := waitForCompletion(e, resp.StatusID); err != nil {
			return UploadOutcome{}, err
		}
	}

	return UploadOutcome{
		CatalogID: resp.CatalogID,
		VersionID: resp.CatalogVersionID,
		Sent:      sent,
	}, nil
}

// buildZip writes a zip archive to a temp file and returns the per-path
// streamed hashes. Buffering on disk keeps very large zips from pinning a
// multi-GiB allocation.
func buildZip(projectDir string, files []FileAction) (string, map[string]FileEntry, error) {
	tmp, err := os.CreateTemp("", "wapi-sync-*.zip")
	if err != nil {
		return "", nil, fmt.Errorf("create zip tempfile: %w", err)
	}

	defer func() { _ = tmp.Close() }()

	zw := zip.NewWriter(tmp)

	defer func() { _ = zw.Close() }()

	sent := make(map[string]FileEntry, len(files))

	for _, fa := range files {
		abs := filepath.Join(projectDir, filepath.FromSlash(fa.Path))

		entry, err := addToZip(zw, abs, fa.Path)
		if err != nil {
			_ = os.Remove(tmp.Name())
			return "", nil, err
		}

		sent[fa.Path] = entry
	}

	if err := zw.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("close zip writer: %w", err)
	}

	return tmp.Name(), sent, nil
}

func addToZip(zw *zip.Writer, src, archivePath string) (FileEntry, error) {
	in, err := os.Open(src)
	if err != nil {
		return FileEntry{}, fmt.Errorf("open %s for zip: %w", src, err)
	}

	defer func() { _ = in.Close() }()

	hdr := &zip.FileHeader{Name: archivePath, Method: zip.Deflate}

	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return FileEntry{}, fmt.Errorf("zip header for %s: %w", archivePath, err)
	}

	// Hash the bytes entering the archive, not the Phase-2 planned hash.
	// MultiWriter mirrors the download verification at download.go: a file
	// rewritten between plan and zip-build must leave BASE describing what
	// the server extracted. The size comes from io.Copy's return, not from
	// fa.LocalSize, so it always describes the bytes that entered the archive.
	h := newStreamHasher()

	n, err := io.Copy(io.MultiWriter(w, h), in)
	if err != nil {
		return FileEntry{}, fmt.Errorf("copy %s into zip: %w", archivePath, err)
	}

	return streamedEntry(h, n), nil
}

// postZip dispatches to UploadFromZipNew (first-sync, no catalog) or
// UploadFromZipExisting (subsequent syncs).
func postZip(e *Engine, body io.Reader, size int64) (*filesapi.FromFileResp, error) {
	if id := resolveExistingCatalogID(e); id != "" {
		return e.files.UploadFromZipExisting(id, "wapi-sync.zip", filesapi.OverwriteReplace, size, body)
	}

	return e.files.UploadFromZipNew("wapi-sync.zip", size, body)
}

// waitForCompletion polls until terminal status or ZipPollTimeoutSecs
// elapses.
func waitForCompletion(e *Engine, statusID string) error {
	deadline := time.Now().Add(time.Duration(ZipPollTimeoutSecs) * time.Second)

	for {
		if time.Now().After(deadline) {
			return errors.New("timeout waiting for archive extract")
		}

		resp, err := e.files.PollStatus(statusID)
		if err != nil {
			return fmt.Errorf("poll status %s: %w", statusID, err)
		}

		if filesapi.IsTerminalStatus(resp.Status) {
			if filesapi.IsErrorStatus(resp.Status) {
				return fmt.Errorf("zip extraction failed: %s (%s)", resp.Status, resp.Message)
			}

			return nil
		}

		time.Sleep(time.Duration(ZipPollIntervalMS) * time.Millisecond)
	}
}
