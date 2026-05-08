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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Fetcher returns a content fetcher backed by the engine's project
// directory and FilesAPI client. Only valid while the engine is alive.
func (e *Engine) Fetcher() *engineFetcher {
	return &engineFetcher{e: e}
}

type engineFetcher struct {
	e *Engine
}

func (f *engineFetcher) LocalContent(path string) ([]byte, error) {
	abs := filepath.Join(f.e.projectDir, filepath.FromSlash(path))

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read local %s: %w", path, err)
	}

	return data, nil
}

func (f *engineFetcher) RemoteContent(path string) ([]byte, error) {
	codeRef := codeRefOrEmpty(f.e)
	if codeRef.CatalogID == "" || codeRef.CatalogVersionID == "" {
		return nil, errors.New("no remote version available (first sync)")
	}

	var buf bytes.Buffer

	_, _, err := f.e.files.DownloadFile(codeRef.CatalogID, codeRef.CatalogVersionID, path, &buf)
	if err != nil {
		return nil, fmt.Errorf("download remote %s: %w", path, err)
	}

	return buf.Bytes(), nil
}
