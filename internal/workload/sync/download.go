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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload/fileops"
)

// downloadFiles is the Engine-bound entry the sync pipeline uses; the reusable
// work lives in DownloadFiles, which needs only a client and a destination.
func downloadFiles(e *Engine, catalogID, versionID string, files []FileAction) error {
	return DownloadFiles(e.files, e.projectDir, catalogID, versionID, files)
}

// DownloadFiles pulls files into dir in parallel up to DownloadConcurrency.
// Each download streams to disk and computes SHA-256 in the same pass so
// post-download verification is free. The first error closes done to stop
// other workers from picking up the next file.
//
// It takes a client and a destination rather than an *Engine so callers
// outside the sync pipeline can reuse it: the first-bind seed in
// internal/workload/up pulls a workload's current code with the same
// verified, partial-safe download the pipeline uses.
func DownloadFiles(client filesapi.Client, dir, catalogID, versionID string, files []FileAction) error {
	if len(files) == 0 {
		return nil
	}

	done := make(chan struct{})

	var cancelOnce sync.Once

	cancel := func() { cancelOnce.Do(func() { close(done) }) }
	defer cancel()

	sem := make(chan struct{}, DownloadConcurrency)
	errCh := make(chan error, len(files))

	var wg sync.WaitGroup

	for _, fa := range files {
		fa := fa

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer recoverWorkerPanic("downloading "+fa.Path, errCh, cancel)

			select {
			case sem <- struct{}{}:
			case <-done:
				return
			}

			defer func() { <-sem }()

			if err := DownloadOne(client, dir, catalogID, versionID, fa); err != nil {
				select {
				case errCh <- err:
					cancel()
				default:
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		return err
	}

	return nil
}

// downloadOne is the Engine-bound entry; the reusable work lives in DownloadOne.
func downloadOne(e *Engine, catalogID, versionID string, fa FileAction) error {
	return DownloadOne(e.files, e.projectDir, catalogID, versionID, fa)
}

// DownloadOne streams the remote file into dir, hashing as it writes, and
// removes the partial file on any failure so a half-written file is never left
// on disk. Empty RemoteHash skips checksum verification (e.g. a synthesized
// EDIT_DEL_CONFLICT, or a caller whose listing carries no hash).
func DownloadOne(client filesapi.Client, dir, catalogID, versionID string, fa FileAction) error {
	// phase5Execute already rejected unsafe server paths up front via
	// validateServerPaths; re-check here so the per-call-site invariant
	// survives future refactors that might bypass the phase entry point.
	if err := fileops.SafeRelPath(fa.Path); err != nil {
		return fmt.Errorf("server returned unsafe download path %q: %w", fa.Path, err)
	}

	dst := filepath.Join(dir, filepath.FromSlash(fa.Path))

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir parent for %s: %w", fa.Path, err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", fa.Path, err)
	}

	h := sha256.New()
	mw := io.MultiWriter(out, h)

	_, n, err := client.DownloadFile(catalogID, versionID, fa.Path, mw)

	closeErr := out.Close()

	if err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("download %s: %w", fa.Path, err)
	}

	if closeErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("close %s: %w", fa.Path, closeErr)
	}

	if fa.RemoteSize > 0 && n != fa.RemoteSize {
		_ = os.Remove(dst)
		return fmt.Errorf("download size mismatch on %s: expected %d, got %d", fa.Path, fa.RemoteSize, n)
	}

	if fa.RemoteHash != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != fa.RemoteHash {
			_ = os.Remove(dst)
			return fmt.Errorf("checksum mismatch on %s: expected %s, got %s", fa.Path, fa.RemoteHash, got)
		}
	}

	return nil
}

// recoverWorkerPanic is a deferred handler for parallel-worker goroutines.
// On panic it logs the value and forwards a synthesized error to errCh so
// the orchestrator returns a real error instead of silent success.
func recoverWorkerPanic(label string, errCh chan<- error, cancel func()) {
	r := recover()
	if r == nil {
		return
	}

	log.Errorf("panic %s: %v", label, r)

	select {
	case errCh <- fmt.Errorf("panic %s: %v", label, r):
		cancel()
	default:
	}
}
