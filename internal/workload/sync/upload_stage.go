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
	"os"
	"path/filepath"
	"sync"

	"github.com/datarobot/cli/internal/drapi/filesapi"
)

// StageUploader implements the stage-based workflow: create catalog if
// missing, create stage, upload each file, apply stage.
type StageUploader struct{}

// ApplyUploads pushes files via the stage workflow.
func (StageUploader) ApplyUploads(e *Engine, files []FileAction) (string, string, error) {
	catalogID, err := ensureCatalog(e)
	if err != nil {
		return "", "", err
	}

	stage, err := e.files.CreateStage(catalogID)
	if err != nil {
		return "", "", fmt.Errorf("create stage: %w", err)
	}

	if err := uploadFilesParallel(e, catalogID, stage.StageID, files); err != nil {
		return "", "", err
	}

	apply, err := e.files.ApplyStage(catalogID, stage.StageID, filesapi.OverwriteReplace)
	if err != nil {
		return "", "", fmt.Errorf("apply stage: %w", err)
	}

	return catalogID, apply.CatalogVersionID, nil
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

// uploadFilesParallel uploads files up to UploadConcurrency. The first
// error closes done to stop other workers from starting; in-flight
// workers still finish their current upload before the function returns.
func uploadFilesParallel(e *Engine, catalogID, stageID string, files []FileAction) error {
	if len(files) == 0 {
		return nil
	}

	done := make(chan struct{})

	var cancelOnce sync.Once

	cancel := func() { cancelOnce.Do(func() { close(done) }) }
	defer cancel()

	sem := make(chan struct{}, UploadConcurrency)
	errCh := make(chan error, len(files))

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

			if err := uploadOneToStage(e, catalogID, stageID, fa); err != nil {
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

func uploadOneToStage(e *Engine, catalogID, stageID string, fa FileAction) error {
	abs := filepath.Join(e.projectDir, filepath.FromSlash(fa.Path))

	f, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("open %s: %w", fa.Path, err)
	}

	defer func() { _ = f.Close() }()

	if err := e.files.UploadToStage(catalogID, stageID, fa.Path, fa.LocalSize, f); err != nil {
		return fmt.Errorf("upload %s: %w", fa.Path, err)
	}

	return nil
}
