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
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload/fileops"
)

// syncZipName is the filename the zip travels under. It names the upload,
// not the catalog: the catalog's own name is a separate parameter, and a
// zip whose contents are extracted leaves nothing behind called this.
const syncZipName = "wapi-sync.zip"

// ZipUploader implements the async zip workflow: build a zip locally,
// POST it to FilesAPI, poll until terminal.
type ZipUploader struct{}

// ApplyUploads zips the files, POSTs, polls until done, and returns the
// per-path streamed hashes so Phase 7 can record what entered the archive.
func (ZipUploader) ApplyUploads(e *Engine, files []FileAction) (UploadOutcome, error) {
	zipPath, sent, err := buildZip(e.projectDir, files, e.files.SupportsExecutable())
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
// supported says whether the server keeps the executable bit; when it does
// not, the sent entries record it as unknown.
//
// The temp file is closed before any failure-path removal: on Windows an
// open handle blocks os.Remove (Go opens files without FILE_SHARE_DELETE,
// so the remove fails with a sharing violation), and the leak-prone order —
// remove-then-close via defers — would strand the archive in the system
// temp dir on every build failure. POSIX unlinks an open file freely, which
// is exactly why the leak only ever showed on Windows.
func buildZip(projectDir string, files []FileAction, supported bool) (string, map[string]FileEntry, error) {
	tmp, err := os.CreateTemp("", "wapi-sync-*.zip")
	if err != nil {
		return "", nil, fmt.Errorf("create zip tempfile: %w", err)
	}

	sent, err := writeZip(tmp, projectDir, files, supported)

	// A close error must not mask the real failure, so it is adopted only
	// when the archive built cleanly.
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		// The remove itself can fail — a Windows sharing violation was the
		// original leak — so a discarded error here would hide exactly the
		// recurrence this cleanup exists to prevent. Log it instead.
		if rmErr := os.Remove(tmp.Name()); rmErr != nil {
			log.Warn("zip temp cleanup failed; the archive may be stranded in the system temp dir",
				"path", tmp.Name(), "err", rmErr)
		}

		return "", nil, err
	}

	return tmp.Name(), sent, nil
}

// writeZip streams every planned file into tmp and returns the per-path
// streamed hashes. It owns no lifecycle: the caller closes the file and
// removes it on failure.
func writeZip(tmp *os.File, projectDir string, files []FileAction, supported bool) (map[string]FileEntry, error) {
	zw := zip.NewWriter(tmp)

	sent := make(map[string]FileEntry, len(files))

	for _, fa := range files {
		abs := filepath.Join(projectDir, filepath.FromSlash(fa.Path))

		entry, err := addToZip(zw, abs, fa)
		if err != nil {
			// Lifecycle contract: zw is deliberately left unclosed here — the caller closes the temp file and removes the dead archive, and closing would only flush a doomed central directory that risks masking the real error.
			return nil, err
		}

		entry.Executable = sentExecutable(entry.Executable, supported)
		sent[fa.Path] = entry
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}

	return sent, nil
}

// addToZip archives one file. The server reads the executable bit from the
// entry's Unix mode, so every entry carries 0755 or 0644, from the bit merged
// with the base and the remote.
func addToZip(zw *zip.Writer, src string, fa FileAction) (FileEntry, error) {
	archivePath := fa.Path

	in, err := os.Open(src)
	if err != nil {
		return FileEntry{}, fmt.Errorf("open %s for zip: %w", src, err)
	}

	defer func() { _ = in.Close() }()

	stat, err := in.Stat()
	if err != nil {
		return FileEntry{}, fmt.Errorf("stat %s for zip: %w", src, err)
	}

	local := fileops.LocalExecutable(stat.Mode())
	exec := mergeExecutable(fa.BaseExec, local, fa.RemoteExec)

	if err := keepLocalExecutable(src, local, exec); err != nil {
		return FileEntry{}, fmt.Errorf("%s: %w", archivePath, err)
	}

	hdr := &zip.FileHeader{Name: archivePath, Method: zip.Deflate}
	hdr.SetMode(fileops.ArchivePerm(isSet(exec)))

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

	entry := streamedEntry(h, n)
	entry.Executable = exec

	return entry, nil
}

// postZip creates the catalog if the project has none, then adds the zip's
// contents to it.
//
// A first sync could instead create and fill in one request, via the
// Files API's create-from-file route, and used to. Creating separately
// keeps the catalog's name on the JSON create call, which is the only
// route where naming is safe against every server: the create-from-file
// route validates its multipart form strictly, so a name reaching a
// server that predates the parameter fails the whole upload, and the
// retry that would paper over it means streaming the entire archive a
// second time. The JSON route ignores what it does not recognize, so
// there the same server just leaves its own default on the entry.
//
// The cost is an extra round trip on first sync and, if the upload then
// fails, an empty catalog with nothing in it. Both are what the stage
// path has always done, so this is one shape rather than two.
func postZip(e *Engine, body io.Reader, size int64) (*filesapi.FromFileResp, error) {
	catalogID, err := ensureCatalog(e)
	if err != nil {
		return nil, err
	}

	return e.files.UploadFromZipExisting(catalogID, syncZipName, filesapi.OverwriteReplace, size, body)
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
