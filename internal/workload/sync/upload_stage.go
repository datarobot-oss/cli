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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/datarobot/cli/internal/drapi/filesapi"
)

// StageUploader implements the stage-based workflow: create catalog if
// missing, create stage, upload each file, apply stage.
type StageUploader struct{}

// ApplyUploads pushes files via the stage workflow and returns the per-path
// streamed hashes so Phase 6 can record what the server actually received.
func (StageUploader) ApplyUploads(e *Engine, files []FileAction) (UploadOutcome, error) {
	catalogID, err := ensureCatalog(e)
	if err != nil {
		return UploadOutcome{}, err
	}

	stage, err := e.files.CreateStage(catalogID)
	if err != nil {
		return UploadOutcome{}, fmt.Errorf("create stage: %w", err)
	}

	sent, err := uploadFilesParallel(e, catalogID, stage.StageID, files)
	if err != nil {
		return UploadOutcome{}, err
	}

	apply, err := e.files.ApplyStage(catalogID, stage.StageID, filesapi.OverwriteReplace)
	if err != nil {
		return UploadOutcome{}, fmt.Errorf("apply stage: %w", err)
	}

	return UploadOutcome{
		CatalogID: catalogID,
		VersionID: apply.CatalogVersionID,
		Sent:      sent,
	}, nil
}

// ensureCatalog returns the catalog ID, creating a new one when neither
// config nor artifact has one (first-sync against an empty artifact).
func ensureCatalog(e *Engine) (string, error) {
	if id := resolveExistingCatalogID(e); id != "" {
		return id, nil
	}

	cat, err := e.files.CreateCatalog()
	if err != nil {
		return "", fmt.Errorf("create catalog: %w", err)
	}

	return cat.CatalogID, nil
}

// uploadResult carries one file's streamed hash and size from a worker
// goroutine to the orchestrator via a buffered result channel, matching the
// existing errCh convention rather than adding a mutex-guarded map.
type uploadResult struct {
	path  string
	entry FileEntry
}

// uploadFilesParallel uploads files up to UploadConcurrency and collects
// per-path streamed hashes. The first error closes done to stop other workers
// from starting; in-flight workers still finish their current upload before
// the function returns. Both errCh and resCh are buffered to len(files) so a
// send never blocks or drops. resCh is closed by the orchestrator only, after
// wg.Wait, following the same convention as errCh.
func uploadFilesParallel(e *Engine, catalogID, stageID string, files []FileAction) (map[string]FileEntry, error) {
	if len(files) == 0 {
		return nil, nil
	}

	done := make(chan struct{})

	var cancelOnce sync.Once

	cancel := func() { cancelOnce.Do(func() { close(done) }) }
	defer cancel()

	sem := make(chan struct{}, UploadConcurrency)
	errCh := make(chan error, len(files))
	resCh := make(chan uploadResult, len(files))

	var wg sync.WaitGroup

	for _, fa := range files {
		fa := fa

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer recoverWorkerPanic("uploading "+fa.Path, errCh, cancel)

			select {
			case sem <- struct{}{}:
			case <-done:
				return
			}

			defer func() { <-sem }()

			entry, err := uploadOneToStage(e, catalogID, stageID, fa)
			if err != nil {
				select {
				case errCh <- err:
					cancel()
				default:
				}

				return
			}

			resCh <- uploadResult{path: fa.Path, entry: entry}
		}()
	}

	wg.Wait()
	close(errCh)
	close(resCh)

	if err := <-errCh; err != nil {
		return nil, err
	}

	sent := make(map[string]FileEntry, len(files))

	for r := range resCh {
		sent[r.path] = r.entry
	}

	return sent, nil
}

func uploadOneToStage(e *Engine, catalogID, stageID string, fa FileAction) (FileEntry, error) {
	abs := filepath.Join(e.projectDir, filepath.FromSlash(fa.Path))

	f, err := os.Open(abs)
	if err != nil {
		return FileEntry{}, fmt.Errorf("open %s: %w", fa.Path, err)
	}

	defer func() { _ = f.Close() }()

	// Content-length from the already-open handle, not a fresh os.Stat: a
	// fresh stat reopens the TOCTOU window and can follow a symlink swapped
	// in since the plan phase. Do not abort merely because this differs from
	// fa.LocalSize — a mid-run edit is legitimate; upload the current bytes
	// and let the recorded hash be the truth.
	stat, err := f.Stat()
	if err != nil {
		return FileEntry{}, fmt.Errorf("stat %s: %w", fa.Path, err)
	}

	size := stat.Size()

	// Hash the exact bytes streamed, not the Phase-2 planned hash. TeeReader
	// is the right primitive: UploadToStage pipes the body through io.Copy,
	// so every byte that reaches the wire passes through the hasher.
	h := newStreamHasher()

	if err := e.files.UploadToStage(catalogID, stageID, fa.Path, size, io.TeeReader(f, h)); err != nil {
		return FileEntry{}, fmt.Errorf("upload %s: %w", fa.Path, err)
	}

	return streamedEntry(h, size), nil
}
